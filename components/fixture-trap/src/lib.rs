wit_bindgen::generate!({
    path: "../../wit",
    world: "plugin",
});

struct Trap;

impl Guest for Trap {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:fixture-trap".into(),
            version: "1.0.0".into(),
            provides: Vec::new(),
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        panic!("intentional component trap")
    }

    fn deactivate() {}

    fn invoke(_command: String, _args: Vec<String>) -> Result<u32, String> {
        Ok(0)
    }
}

export!(Trap);
