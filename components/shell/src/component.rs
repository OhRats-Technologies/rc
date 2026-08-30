wit_bindgen::generate!({
    path: "../../wit",
    world: "shell",
    generate_all,
});

use crate::ast;
use exports::ohrats::rc_shell::compiler::Guest as CompilerGuest;
use ohrats::{
    rc_plugin::types::Service,
    rc_shell::types::{
        Assignment, Chain, Command, Connector, Pipeline, Redirect, RedirectMode, RedirectStream,
        Script, Word, WordPart, WordPartKind,
    },
};

mod executor;

pub(super) struct Shell;

impl Guest for Shell {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:shell".into(),
            version: "0.1.0".into(),
            provides: ["compiler", "executor"]
                .into_iter()
                .map(|name| Service {
                    name: format!("ohrats:rc-shell/{name}"),
                    version: "0.1.0".into(),
                    priority: 100,
                    keys: Vec::new(),
                })
                .collect(),
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl CompilerGuest for Shell {
    fn compile(source: String) -> Result<Script, String> {
        crate::parse(&source)
            .map(convert_script)
            .map_err(|error| error.to_string())
    }
}

fn convert_script(value: ast::Script) -> Script {
    Script {
        chains: value.chains.into_iter().map(convert_chain).collect(),
    }
}

fn convert_chain(value: ast::Chain) -> Chain {
    Chain {
        pipeline: Pipeline {
            commands: value
                .pipeline
                .commands
                .into_iter()
                .map(convert_command)
                .collect(),
        },
        next: value.next.map(|next| match next {
            ast::Connector::Always => Connector::Always,
            ast::Connector::And => Connector::And,
            ast::Connector::Or => Connector::Or,
        }),
    }
}

fn convert_command(value: ast::Command) -> Command {
    Command {
        assignments: value
            .assignments
            .into_iter()
            .map(|assignment| Assignment {
                name: assignment.name,
                value: convert_word(assignment.value),
            })
            .collect(),
        words: value.words.into_iter().map(convert_word).collect(),
        redirects: value.redirects.into_iter().map(convert_redirect).collect(),
    }
}

fn convert_word(value: ast::Word) -> Word {
    Word {
        parts: value
            .parts
            .into_iter()
            .map(|part| match part {
                ast::WordPart::Literal(value) => word_part(WordPartKind::Literal, value),
                ast::WordPart::Variable(value) => word_part(WordPartKind::Variable, value),
                ast::WordPart::CommandSubstitution(value) => {
                    word_part(WordPartKind::CommandSubstitution, value)
                }
                ast::WordPart::Glob(value) => word_part(WordPartKind::Glob, value),
            })
            .collect(),
    }
}

fn word_part(kind: WordPartKind, value: String) -> WordPart {
    WordPart { kind, value }
}

fn convert_redirect(value: ast::Redirect) -> Redirect {
    let stream = match value.stream {
        ast::RedirectStream::Stdin => RedirectStream::Stdin,
        ast::RedirectStream::Stdout => RedirectStream::Stdout,
        ast::RedirectStream::Stderr => RedirectStream::Stderr,
        ast::RedirectStream::StdoutAndStderr => RedirectStream::StdoutAndStderr,
    };
    let mode = match value.mode {
        ast::RedirectMode::Read => RedirectMode::Read,
        ast::RedirectMode::Write => RedirectMode::Write,
        ast::RedirectMode::Append => RedirectMode::Append,
    };
    Redirect {
        target_stream: stream,
        mode,
        target: convert_word(value.target),
    }
}

export!(Shell);
