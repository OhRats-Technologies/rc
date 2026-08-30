#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Script {
    pub chains: Vec<Chain>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Chain {
    pub pipeline: Pipeline,
    pub next: Option<Connector>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Connector {
    Always,
    And,
    Or,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Pipeline {
    pub commands: Vec<Command>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Command {
    pub assignments: Vec<Assignment>,
    pub words: Vec<Word>,
    pub redirects: Vec<Redirect>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Assignment {
    pub name: String,
    pub value: Word,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Word {
    pub parts: Vec<WordPart>,
}

impl Word {
    pub fn literal(value: impl Into<String>) -> Self {
        Self {
            parts: vec![WordPart::Literal(value.into())],
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum WordPart {
    Literal(String),
    Variable(String),
    CommandSubstitution(String),
    Glob(String),
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Redirect {
    pub stream: RedirectStream,
    pub mode: RedirectMode,
    pub target: Word,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RedirectStream {
    Stdin,
    Stdout,
    Stderr,
    StdoutAndStderr,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RedirectMode {
    Read,
    Write,
    Append,
}
