use async_trait::async_trait;
use parking_lot::Mutex;
use rc_context::{
    Activation, Component, ComponentState, Context, EffectScope, Runtime, ServiceKey,
};
use std::sync::Arc;

struct Dependency;
struct Output;

struct ExampleComponent {
    log: Arc<Mutex<Vec<&'static str>>>,
    fail: bool,
}

#[async_trait]
impl Component for ExampleComponent {
    fn name(&self) -> &'static str {
        "example"
    }

    fn requirements(&self) -> Vec<ServiceKey> {
        vec![ServiceKey::of::<Dependency>()]
    }

    async fn activate(&self, _: &Context, activation: &mut Activation) -> anyhow::Result<()> {
        self.log.lock().push("activate");
        let log = self.log.clone();
        activation
            .effects
            .defer(move || log.lock().push("deactivate"));
        if self.fail {
            anyhow::bail!("activation failed");
        }
        activation.provide(Arc::new(Output));
        Ok(())
    }
}

#[tokio::test]
async fn dependencies_drive_activation_and_cleanup() {
    let context = Context::root("test");
    let log = Arc::new(Mutex::new(Vec::new()));
    let mut runtime = Runtime::new(context.clone());
    runtime
        .register(Arc::new(ExampleComponent {
            log: log.clone(),
            fail: false,
        }))
        .unwrap();

    runtime.reconcile().await.unwrap();
    assert_eq!(runtime.state("example"), Some(ComponentState::Waiting));
    assert!(context.get::<Output>().is_none());

    let dependency = context.provide(Arc::new(Dependency)).unwrap();
    runtime.reconcile().await.unwrap();
    assert_eq!(runtime.state("example"), Some(ComponentState::Active));
    assert!(context.get::<Output>().is_some());

    drop(dependency);
    runtime.reconcile().await.unwrap();
    assert_eq!(runtime.state("example"), Some(ComponentState::Waiting));
    assert!(context.get::<Output>().is_none());
    assert_eq!(&*log.lock(), &["activate", "deactivate"]);
}

#[tokio::test]
async fn failed_replacement_restores_the_previous_component() {
    let context = Context::root("replace");
    let _dependency = context.provide(Arc::new(Dependency)).unwrap();
    let log = Arc::new(Mutex::new(Vec::new()));
    let mut runtime = Runtime::new(context.clone());
    runtime
        .register(Arc::new(ExampleComponent {
            log: log.clone(),
            fail: false,
        }))
        .unwrap();
    runtime.reconcile().await.unwrap();

    let error = runtime
        .replace(
            "example",
            Arc::new(ExampleComponent {
                log: log.clone(),
                fail: true,
            }),
        )
        .await
        .unwrap_err();
    assert!(error.to_string().contains("activation failed"));
    assert_eq!(runtime.state("example"), Some(ComponentState::Active));
    assert!(context.get::<Output>().is_some());
    assert_eq!(
        &*log.lock(),
        &[
            "activate",
            "deactivate",
            "activate",
            "deactivate",
            "activate"
        ]
    );
}

#[tokio::test]
async fn partial_activation_effects_revert_on_failure() {
    let context = Context::root("failure");
    let _dependency = context.provide(Arc::new(Dependency)).unwrap();
    let log = Arc::new(Mutex::new(Vec::new()));
    let mut runtime = Runtime::new(context);
    runtime
        .register(Arc::new(ExampleComponent {
            log: log.clone(),
            fail: true,
        }))
        .unwrap();

    assert!(runtime.reconcile().await.is_err());
    assert_eq!(&*log.lock(), &["activate", "deactivate"]);
}

#[tokio::test]
async fn effects_revert_in_reverse_order() {
    let log = Arc::new(Mutex::new(Vec::new()));
    let mut effects = EffectScope::new();
    for value in [1, 2, 3] {
        let log = log.clone();
        effects.defer(move || log.lock().push(value));
    }
    effects.revert().await;
    assert_eq!(&*log.lock(), &[3, 2, 1]);
}

#[test]
fn child_realms_inherit_parent_but_not_siblings() {
    struct Root;
    struct Local;
    let root = Context::root("root");
    let _root = root.provide(Arc::new(Root)).unwrap();
    let left = root.child("left");
    let right = root.child("right");
    let _left = left.provide(Arc::new(Local)).unwrap();

    assert!(left.get::<Root>().is_some());
    assert!(right.get::<Root>().is_some());
    assert!(left.get::<Local>().is_some());
    assert!(right.get::<Local>().is_none());
}
