wasmtime::component::bindgen!({
    path: "../wit",
    world: "kernel-host",
});

pub use KernelHost as Plugin;
