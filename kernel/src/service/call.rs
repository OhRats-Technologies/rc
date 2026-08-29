use super::PinnedProvider;
use crate::host;
use std::cell::RefCell;
use wasmtime::component::Val;

pub(super) fn provider_owned(
    provider: &PinnedProvider,
    service: &str,
    function: &str,
    params: &[Val],
) -> wasmtime::Result<Vec<Val>> {
    let count = provider.provider.handle.with_active(|active| {
        let func = *active
            .exports
            .get(&(service.to_owned(), function.to_owned()))
            .ok_or_else(|| wasmtime::format_err!("provider is missing {service}#{function}"))?;
        Ok(func.ty(&active.store).results().len())
    })?;
    let mut results = vec![Val::Bool(false); count];
    self::provider(provider, None, service, function, params, &mut results)?;
    Ok(results)
}

pub(super) fn provider(
    provider: &PinnedProvider,
    caller: Option<&str>,
    service: &str,
    function: &str,
    params: &[Val],
    results: &mut [Val],
) -> wasmtime::Result<()> {
    let key = format!("{}#{service}#{function}", provider.component_id());
    let _guard = CallGuard::enter(key)?;
    provider.provider.handle.with_active(|active| {
        let func = *active
            .exports
            .get(&(service.to_owned(), function.to_owned()))
            .ok_or_else(|| wasmtime::format_err!("provider is missing {service}#{function}"))?;
        let _context = caller.map(|caller| active.store.data().push_caller(caller.to_owned()));
        active.store.set_fuel(host::SERVICE_FUEL)?;
        func.call(&mut active.store, params, results)
    })
}

thread_local! {
    static CALL_STACK: RefCell<Vec<String>> = const { RefCell::new(Vec::new()) };
}

struct CallGuard;

impl CallGuard {
    fn enter(key: String) -> wasmtime::Result<Self> {
        CALL_STACK.with(|stack| {
            let mut stack = stack.borrow_mut();
            if stack.contains(&key) {
                return Err(wasmtime::format_err!(
                    "component service cycle detected at {key}"
                ));
            }
            stack.push(key);
            Ok(Self)
        })
    }
}

impl Drop for CallGuard {
    fn drop(&mut self) {
        CALL_STACK.with(|stack| {
            stack.borrow_mut().pop();
        });
    }
}
