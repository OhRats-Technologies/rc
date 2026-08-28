use super::ProcessManager;

impl ProcessManager {
    pub fn relay_process(&self, relay_id: &str) -> Option<String> {
        self.processes
            .lock()
            .iter()
            .find_map(|(id, process)| (process.relay_id == relay_id).then(|| id.clone()))
    }

    pub fn relay_process_ids(&self) -> Vec<String> {
        self.processes
            .lock()
            .iter()
            .filter(|(_, process)| !process.relay_id.is_empty())
            .map(|(id, _)| id.clone())
            .collect()
    }
}
