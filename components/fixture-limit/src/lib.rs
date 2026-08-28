wit_bindgen::generate!({
    path: "../../wit",
    world: "plugin",
});

struct Limit;

impl Guest for Limit {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:fixture-limit".into(),
            version: "1.0.0".into(),
            provides: Vec::new(),
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        let mut bytes = Vec::new();
        bytes.resize(80 * 1024 * 1024, 1_u8);
        std::hint::black_box(bytes);
        Ok(())
    }

    fn deactivate() {}

    fn invoke(_command: String, _args: Vec<String>) -> Result<u32, String> {
        Ok(0)
    }
}

export!(Limit);
