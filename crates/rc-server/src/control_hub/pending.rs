use super::{ControlHub, ControlReply, ControlSignalError, Pending, PendingKind};
use parking_lot::Mutex;
use rc_protocol::{IceServer, ServerToNode};
use std::{sync::Arc, time::Duration};

const SIGNAL_TIMEOUT: Duration = Duration::from_secs(10);

impl ControlHub {
    pub(super) async fn request(
        &self,
        request_id: &str,
        mut pending: Pending,
        message: ServerToNode,
    ) -> Result<ControlReply, ControlSignalError> {
        let (tx, rx) = tokio::sync::oneshot::channel();
        *pending.sender.get_mut() = Some(tx);
        let device_id = pending.device_id.clone();
        self.inner
            .pending
            .insert(request_id.to_owned(), Arc::new(pending));
        if self.inner.nodes.send(&device_id, &message).await.is_err() {
            self.inner.pending.remove(request_id);
            return Err(ControlSignalError::Offline);
        }
        match tokio::time::timeout(SIGNAL_TIMEOUT, rx).await {
            Ok(Ok(result)) => result,
            Ok(Err(_)) => Err(ControlSignalError::Disconnected),
            Err(_) => {
                self.inner.pending.remove(request_id);
                Err(ControlSignalError::Timeout)
            }
        }
    }

    pub(super) fn finish(
        &self,
        request_id: &str,
        device_id: &str,
        kind: PendingKind,
        result: Result<ControlReply, ControlSignalError>,
    ) -> bool {
        let Some(pending) = self.pending(request_id, device_id, kind) else {
            return true;
        };
        self.complete(request_id, pending, result);
        true
    }
    pub(super) fn pending(
        &self,
        request_id: &str,
        device_id: &str,
        kind: PendingKind,
    ) -> Option<Arc<Pending>> {
        self.inner
            .pending
            .get(request_id)
            .filter(|pending| pending.device_id == device_id && pending.kind == kind)
            .map(|pending| pending.clone())
    }
    pub(super) fn pending_any(&self, request_id: &str, device_id: &str) -> Option<Arc<Pending>> {
        self.inner
            .pending
            .get(request_id)
            .filter(|pending| pending.device_id == device_id)
            .map(|pending| pending.clone())
    }
    pub(super) fn complete(
        &self,
        request_id: &str,
        pending: Arc<Pending>,
        result: Result<ControlReply, ControlSignalError>,
    ) {
        self.inner.pending.remove(request_id);
        send(&pending, result);
    }
}

pub(super) fn make(
    kind: PendingKind,
    device_id: &str,
    user_id: &str,
    client_id: &str,
    ice_servers: Vec<IceServer>,
) -> Pending {
    Pending {
        kind,
        device_id: device_id.to_owned(),
        user_id: user_id.to_owned(),
        client_id: client_id.to_owned(),
        ice_servers,
        sender: Mutex::new(None),
    }
}
pub(super) fn send(pending: &Pending, result: Result<ControlReply, ControlSignalError>) {
    if let Some(sender) = pending.sender.lock().take() {
        let _ = sender.send(result);
    }
}
