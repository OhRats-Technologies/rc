use super::*;

fn long_running(terminal: Option<Terminal>) -> SpawnRequest {
    request(&["/D", "/C", "ping -t 127.0.0.1 >nul"], terminal)
}

#[test]
fn nonterminal_terminate_delivers_a_console_group_event() {
    let mut group = Group::new().unwrap();
    let spawned = spawn(&mut group, long_running(None)).unwrap();
    std::thread::sleep(std::time::Duration::from_millis(250));
    group.signal(Signal::Terminate).unwrap();
    assert!(wait(&mut group, spawned.native_child).code.is_some());
}

#[test]
fn conpty_interrupt_delivers_ctrl_c_to_the_terminal_process() {
    let mut group = Group::new().unwrap();
    let terminal = Terminal {
        cols: 80,
        rows: 24,
        term: "xterm-256color".into(),
    };
    let spawned = spawn(&mut group, long_running(Some(terminal))).unwrap();
    std::thread::sleep(std::time::Duration::from_millis(250));
    group.signal(Signal::Interrupt).unwrap();
    assert!(wait(&mut group, spawned.native_child).code.is_some());
}
