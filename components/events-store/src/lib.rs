wit_bindgen::generate!({ path: "../../wit", world: "events-store", generate_all });

mod model;
mod query;
mod storage;
mod store;
mod validate;

use exports::ohrats::rc_events::{
    append::Guest as AppendGuest,
    feed::Guest as FeedGuest,
    query::Guest as QueryGuest,
    retention::{Guest as RetentionGuest, Policy},
};
use ohrats::{
    rc_events::types::{AppendRequest, Authorization, Batch, Event, Filter},
    rc_plugin::types::Service,
};

struct EventsStore;

impl Guest for EventsStore {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:events-store".into(),
            version: "0.1.0".into(),
            provides: ["append", "query", "feed", "retention"]
                .into_iter()
                .map(|name| service(&format!("ohrats:rc-events/{name}")))
                .collect(),
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }
    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl AppendGuest for EventsStore {
    fn append(request: AppendRequest) -> Result<Event, String> {
        store::append(request)
    }
}
impl QueryGuest for EventsStore {
    fn query(auth: Authorization, after: u64, limit: u32, filter: Filter) -> Result<Batch, String> {
        query::run(auth, after, limit, filter)
    }
}
impl FeedGuest for EventsStore {
    fn poll(auth: Authorization, after: u64, limit: u32, filter: Filter) -> Result<Batch, String> {
        query::run(auth, after, limit, filter)
    }
}
impl RetentionGuest for EventsStore {
    fn configure(policy: Policy) -> Result<(), String> {
        store::configure(policy.maximum_events)
    }
    fn current() -> Result<Policy, String> {
        Ok(Policy {
            maximum_events: store::policy()?,
        })
    }
}

fn service(name: &str) -> Service {
    Service {
        name: name.into(),
        version: "0.1.0".into(),
        priority: 100,
        keys: Vec::new(),
    }
}
export!(EventsStore);
