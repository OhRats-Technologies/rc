mod journal;
mod native;
mod resolve;

wit_bindgen::generate!({
    path: "../../wit",
    world: "execution-runtime",
    generate_all,
});

mod component;
mod diagnostics;

use component::ExecutionRuntime;
export!(ExecutionRuntime);
mod leases;
mod registry;
mod replay;
