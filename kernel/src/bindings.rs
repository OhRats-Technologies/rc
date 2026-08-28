wasmtime::component::bindgen!({
    path: "../wit",
    world: "kernel-plugin",
});

pub use KernelPlugin as Plugin;
