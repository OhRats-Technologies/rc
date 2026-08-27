use crate::{AppState, browser_user, workspace_role};
use axum::{
    Router,
    extract::State,
    http::{HeaderMap, StatusCode},
    response::{
        IntoResponse, Response,
        sse::{Event, KeepAlive, Sse},
    },
    routing::get,
};
use futures_util::stream;
use std::{convert::Infallible, time::Duration};

pub fn routes() -> Router<AppState> {
    Router::new().route("/api/v1/events", get(events))
}

async fn events(State(state): State<AppState>, headers: HeaderMap) -> Result<Response, EventError> {
    let user = browser_user(&state, &headers)
        .map_err(|_| EventError)?
        .ok_or(EventError)?;
    let receiver = state.events.subscribe();
    let stream_state = (receiver, state.clone(), user.id);
    let output = stream::unfold(stream_state, |(mut receiver, state, user_id)| async move {
        loop {
            match receiver.recv().await {
                Ok(event) => {
                    if allowed(&state, &user_id, &event) {
                        let data = serde_json::to_string(&event).unwrap_or_else(|_| "{}".into());
                        return Some((
                            Ok::<Event, Infallible>(Event::default().data(data)),
                            (receiver, state, user_id),
                        ));
                    }
                }
                Err(tokio::sync::broadcast::error::RecvError::Lagged(skipped)) => {
                    tracing::warn!(skipped, %user_id, "closing lagged event stream for resync");
                    return None;
                }
                Err(tokio::sync::broadcast::error::RecvError::Closed) => return None,
            }
        }
    });
    Ok(Sse::new(output)
        .keep_alive(
            KeepAlive::new()
                .interval(Duration::from_secs(20))
                .text("keepalive"),
        )
        .into_response())
}

fn allowed(state: &AppState, user_id: &str, event: &crate::RcEvent) -> bool {
    let Some(workspace) = event.workspace_id.as_deref() else {
        return false;
    };
    let Ok(role) = workspace_role(state, user_id, workspace) else {
        return false;
    };
    let Some(role) = role else { return false };
    !event.kind.starts_with("process.") || matches!(role.as_str(), "owner" | "operator")
}

struct EventError;
impl IntoResponse for EventError {
    fn into_response(self) -> Response {
        (StatusCode::UNAUTHORIZED, "authentication required").into_response()
    }
}
