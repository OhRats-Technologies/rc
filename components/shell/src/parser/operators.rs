use super::*;

impl Lexer<'_> {
    pub(super) fn operator(&mut self) -> Option<Token> {
        let options = [
            (
                "2>&",
                Token::Redirect(RedirectStream::Stderr, RedirectMode::Duplicate),
            ),
            (
                "1>&",
                Token::Redirect(RedirectStream::Stdout, RedirectMode::Duplicate),
            ),
            (
                ">&",
                Token::Redirect(RedirectStream::Stdout, RedirectMode::Duplicate),
            ),
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
}
