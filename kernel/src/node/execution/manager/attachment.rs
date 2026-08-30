use super::ComponentExecutionManager;
use rc_node::ProcessEvent;

pub(super) fn attach(manager: &ComponentExecutionManager, id: &str, session: &str) -> bool {
    let Some(process) = manager.processes.lock().get(id).cloned() else {
        return false;
    };
    if !process.secure || process.execution.lock().attach(session).is_err() {
        return false;
    }
    *process.writer.lock() = session.into();
    let Some(read) = process.execution.lock().read(0, u32::MAX).ok() else {
        return false;
    };
    for chunk in read.chunks {
        manager.emit(
            &process,
            ProcessEvent::output(chunk.stream, id, &chunk.bytes),
        );
    }
    true
}

pub(super) fn detach(manager: &ComponentExecutionManager, session: &str) {
    let processes = manager
        .processes
        .lock()
        .values()
        .cloned()
        .collect::<Vec<_>>();
    for process in processes {
        if !process.secure || *process.writer.lock() != session {
            continue;
        }
        process.writer.lock().clear();
        let _ = process.execution.lock().detach(session);
    }
}
