# Bun Shell provenance

RC's portable shell semantics and parser conformance corpus are derived from
Bun Shell at commit `ed950b88ab2ec6b58bccdfe7d310731b8ca13c4d`
(2026-08-29):

<https://github.com/oven-sh/bun/tree/ed950b88ab2ec6b58bccdfe7d310731b8ca13c4d>

Upstream is MIT licensed. The adjacent `LICENSE` applies to material ported
from Bun.

## Extraction audit

The pinned parser is in `src/shell_parser/{parse,braces,error,json_fmt}.rs`.
It has the desired AST, lexer, redirects, pipelines, substitutions, expansion,
and Windows-specific redirect flags, but is not a stable-Rust drop-in:

- `lib.rs` enables `adt_const_params`, `generic_const_exprs`, and
  `allocator_api`;
- parser storage depends on `bun_alloc` arena collections;
- strings, Unicode conversion, enum derives, formatting, and logging depend on
  `bun_core`;
- JavaScript interpolation is represented by `JSValueRaw`;
- the executor under `src/runtime/shell` depends on Bun runtime/JSC state,
  event-loop dispatch, and Bun subprocess machinery.

RC therefore ports the parser/AST behavior to stable Rust and executes it
against explicit RC process, filesystem, and environment capabilities. It does
not import JSC, Bun's event loop, or Bun's native subprocess implementation.
RC integration remains under `components/shell`; this directory contains only
upstream provenance and any substantially copied parser/test material.

## Upstream behavioral areas tracked

- parser/AST: `src/shell_parser/parse.rs`
- brace expansion: `src/shell_parser/braces.rs`
- expansion: `src/runtime/shell/states/Expansion.rs`
- pipelines/redirection: `src/runtime/shell/states/{Pipeline,Cmd}.rs`
- subprocesses: `src/runtime/shell/subproc.rs`
- portable builtins: `src/runtime/shell/builtin/`
- Windows behavior: parser redirect flags plus Bun's Windows subprocess layer
