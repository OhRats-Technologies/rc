use super::{ParseError, Token};
use crate::{Assignment, Chain, Command, Connector, Pipeline, Redirect, Script, Word, WordPart};

pub(super) fn parse_tokens(tokens: Vec<Token>) -> Result<Script, ParseError> {
    Parser::new(tokens).script()
}

struct Parser {
    tokens: Vec<Token>,
    position: usize,
}

impl Parser {
    fn new(tokens: Vec<Token>) -> Self {
        Self {
            tokens,
            position: 0,
        }
    }

    fn script(mut self) -> Result<Script, ParseError> {
        let mut chains = Vec::new();
        while self.position < self.tokens.len() {
            let pipeline = self.pipeline()?;
            let next = match self.peek() {
                Some(Token::And) => Some(Connector::And),
                Some(Token::Or) => Some(Connector::Or),
                Some(Token::Semi) => Some(Connector::Always),
                None => None,
                _ => return Err(self.error("expected shell connector")),
            };
            if next.is_some() {
                self.position += 1;
            }
            chains.push(Chain { pipeline, next });
        }
        if chains.is_empty() {
            return Err(self.error("shell script is empty"));
        }
        Ok(Script { chains })
    }

    fn pipeline(&mut self) -> Result<Pipeline, ParseError> {
        let mut commands = vec![self.command()?];
        while matches!(self.peek(), Some(Token::Pipe)) {
            self.position += 1;
            commands.push(self.command()?);
        }
        Ok(Pipeline { commands })
    }

    fn command(&mut self) -> Result<Command, ParseError> {
        let mut words = Vec::new();
        let mut redirects = Vec::new();
        while let Some(token) = self.peek().cloned() {
            match token {
                Token::Word(word) => {
                    self.position += 1;
                    words.push(word);
                }
                Token::Redirect(stream, mode) => {
                    self.position += 1;
                    let Some(Token::Word(target)) = self.peek().cloned() else {
                        return Err(self.error("redirection requires a target"));
                    };
                    self.position += 1;
                    redirects.push(Redirect {
                        stream,
                        mode,
                        target,
                    });
                }
                Token::Pipe | Token::And | Token::Or | Token::Semi => break,
            }
        }
        let mut assignments = Vec::new();
        while words.first().is_some_and(is_assignment) {
            assignments.push(split_assignment(words.remove(0)));
        }
        if words.is_empty() && assignments.is_empty() {
            return Err(self.error("expected command"));
        }
        Ok(Command {
            assignments,
            words,
            redirects,
        })
    }

    fn peek(&self) -> Option<&Token> {
        self.tokens.get(self.position)
    }

    fn error(&self, message: &'static str) -> ParseError {
        ParseError {
            offset: self.position,
            message,
        }
    }
}

fn is_assignment(word: &Word) -> bool {
    let Some(WordPart::Literal(value)) = word.parts.first() else {
        return false;
    };
    value
        .split_once('=')
        .is_some_and(|(name, _)| valid_name(name))
}

fn split_assignment(mut word: Word) -> Assignment {
    let WordPart::Literal(first) = word.parts.remove(0) else {
        unreachable!()
    };
    let (name, value) = first.split_once('=').expect("validated assignment");
    let mut parts = Vec::new();
    if !value.is_empty() {
        parts.push(WordPart::Literal(value.into()));
    }
    parts.extend(word.parts);
    Assignment {
        name: name.into(),
        value: Word { parts },
    }
}

fn valid_name(value: &str) -> bool {
    let mut chars = value.chars();
    chars
        .next()
        .is_some_and(|value| value == '_' || value.is_ascii_alphabetic())
        && chars.all(|value| value == '_' || value.is_ascii_alphanumeric())
}
