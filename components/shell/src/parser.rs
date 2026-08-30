use crate::{RedirectMode, RedirectStream, Script, Word, WordPart};
mod syntax;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ParseError {
    pub offset: usize,
    pub message: &'static str,
}

impl std::fmt::Display for ParseError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(formatter, "{} at byte {}", self.message, self.offset)
    }
}

impl std::error::Error for ParseError {}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(super) enum Token {
    Word(Word),
    Pipe,
    And,
    Or,
    Semi,
    Redirect(RedirectStream, RedirectMode),
}

pub fn parse(source: &str) -> Result<Script, ParseError> {
    let tokens = Lexer::new(source).tokens()?;
    syntax::parse_tokens(tokens)
}

struct Lexer<'a> {
    source: &'a str,
    position: usize,
}

impl<'a> Lexer<'a> {
    fn new(source: &'a str) -> Self {
        Self {
            source,
            position: 0,
        }
    }

    fn tokens(mut self) -> Result<Vec<Token>, ParseError> {
        let mut tokens = Vec::new();
        while self.skip_space() {
            if let Some(token) = self.operator() {
                tokens.push(token);
            } else {
                tokens.push(Token::Word(self.word()?));
            }
        }
        Ok(tokens)
    }

    fn skip_space(&mut self) -> bool {
        while self.rest().chars().next().is_some_and(char::is_whitespace) {
            self.bump();
        }
        self.position < self.source.len()
    }

    fn operator(&mut self) -> Option<Token> {
        let options = [
            (
                "2>>",
                Token::Redirect(RedirectStream::Stderr, RedirectMode::Append),
            ),
            (
                "2>",
                Token::Redirect(RedirectStream::Stderr, RedirectMode::Write),
            ),
            (
                "&>>",
                Token::Redirect(RedirectStream::StdoutAndStderr, RedirectMode::Append),
            ),
            (
                "&>",
                Token::Redirect(RedirectStream::StdoutAndStderr, RedirectMode::Write),
            ),
            (
                ">>",
                Token::Redirect(RedirectStream::Stdout, RedirectMode::Append),
            ),
            ("&&", Token::And),
            ("||", Token::Or),
            ("|", Token::Pipe),
            (";", Token::Semi),
            (
                ">",
                Token::Redirect(RedirectStream::Stdout, RedirectMode::Write),
            ),
            (
                "<",
                Token::Redirect(RedirectStream::Stdin, RedirectMode::Read),
            ),
        ];
        for (source, token) in options {
            if self.rest().starts_with(source) {
                self.position += source.len();
                return Some(token);
            }
        }
        None
    }

    fn word(&mut self) -> Result<Word, ParseError> {
        let mut parts = Vec::new();
        let mut literal = String::new();
        let mut quoted = false;
        while let Some(value) = self.rest().chars().next() {
            if value.is_whitespace() || self.starts_operator() {
                break;
            }
            match value {
                '\\' => {
                    self.bump();
                    literal.push(self.take().ok_or_else(|| self.error("trailing escape"))?);
                }
                '\'' => {
                    quoted = true;
                    self.quoted('\'', &mut literal)?;
                }
                '"' => {
                    quoted = true;
                    self.double_quoted(&mut literal, &mut parts)?;
                }
                '$' => self.dollar(&mut literal, &mut parts)?,
                '*' | '?' | '[' => {
                    flush(&mut literal, &mut parts);
                    parts.push(WordPart::Glob(self.take_glob()));
                }
                _ => {
                    literal.push(value);
                    self.bump();
                }
            }
        }
        flush(&mut literal, &mut parts);
        if parts.is_empty() && quoted {
            parts.push(WordPart::Literal(String::new()));
        }
        if parts.is_empty() {
            return Err(self.error("expected shell word"));
        }
        Ok(Word { parts })
    }

    fn quoted(&mut self, quote: char, literal: &mut String) -> Result<(), ParseError> {
        self.bump();
        while let Some(value) = self.take() {
            if value == quote {
                return Ok(());
            }
            literal.push(value);
        }
        Err(self.error("unterminated quote"))
    }

    fn double_quoted(
        &mut self,
        literal: &mut String,
        parts: &mut Vec<WordPart>,
    ) -> Result<(), ParseError> {
        self.bump();
        loop {
            match self.rest().chars().next() {
                Some('"') => {
                    self.bump();
                    return Ok(());
                }
                Some('$') => self.dollar(literal, parts)?,
                Some('\\') => {
                    self.bump();
                    literal.push(self.take().ok_or_else(|| self.error("trailing escape"))?);
                }
                Some(value) => {
                    literal.push(value);
                    self.bump();
                }
                None => return Err(self.error("unterminated quote")),
            }
        }
    }

    fn dollar(
        &mut self,
        literal: &mut String,
        parts: &mut Vec<WordPart>,
    ) -> Result<(), ParseError> {
        self.bump();
        flush(literal, parts);
        if self.rest().starts_with('(') {
            self.bump();
            let source = self.balanced()?;
            if source.trim().is_empty() {
                return Err(self.error("empty command substitution"));
            }
            parts.push(WordPart::CommandSubstitution(source));
            return Ok(());
        }
        let braced = self.rest().starts_with('{');
        if braced {
            self.bump();
        }
        let name = self.take_name();
        if name.is_empty() || (braced && self.take() != Some('}')) {
            return Err(self.error("invalid variable expansion"));
        }
        parts.push(WordPart::Variable(name));
        Ok(())
    }

    fn balanced(&mut self) -> Result<String, ParseError> {
        let start = self.position;
        let mut depth = 1_u32;
        let mut quote = None;
        while let Some(value) = self.take() {
            if let Some(active) = quote {
                if value == active {
                    quote = None;
                } else if active == '"' && value == '\\' {
                    self.take();
                }
                continue;
            }
            match value {
                '\\' => drop(self.take()),
                '\'' | '"' => quote = Some(value),
                '(' => depth += 1,
                ')' => {
                    depth -= 1;
                    if depth == 0 {
                        return Ok(self.source[start..self.position - 1].into());
                    }
                }
                _ => {}
            }
        }
        Err(self.error("unterminated command substitution"))
    }

    fn take_name(&mut self) -> String {
        let start = self.position;
        while self
            .rest()
            .chars()
            .next()
            .is_some_and(|value| value == '_' || value.is_ascii_alphanumeric())
        {
            self.bump();
        }
        self.source[start..self.position].into()
    }

    fn take_glob(&mut self) -> String {
        let start = self.position;
        while let Some(value) = self.rest().chars().next() {
            if value.is_whitespace() || self.starts_operator() || matches!(value, '$' | '\'' | '"')
            {
                break;
            }
            self.bump();
        }
        self.source[start..self.position].into()
    }

    fn starts_operator(&self) -> bool {
        self.rest().starts_with(['|', ';', '>', '<']) || self.rest().starts_with("&>")
    }

    fn take(&mut self) -> Option<char> {
        let value = self.rest().chars().next()?;
        self.position += value.len_utf8();
        Some(value)
    }

    fn bump(&mut self) {
        let _ = self.take();
    }

    fn rest(&self) -> &'a str {
        &self.source[self.position..]
    }

    fn error(&self, message: &'static str) -> ParseError {
        ParseError {
            offset: self.position,
            message,
        }
    }
}

fn flush(literal: &mut String, parts: &mut Vec<WordPart>) {
    if !literal.is_empty() {
        parts.push(WordPart::Literal(std::mem::take(literal)));
    }
}
