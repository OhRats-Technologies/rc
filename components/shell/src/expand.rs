use crate::{Word, WordPart};

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ExpandError {
    Command(String),
    Glob(String),
}

pub trait ExpansionHost {
    fn environment(&self, name: &str) -> Option<String>;
    fn command_substitution(&mut self, source: &str) -> Result<Vec<u8>, String>;
    fn glob(&self, pattern: &str) -> Result<Vec<String>, String>;
}

pub fn expand_word(word: &Word, host: &mut impl ExpansionHost) -> Result<Vec<String>, ExpandError> {
    let mut values = vec![String::new()];
    for part in &word.parts {
        match part {
            WordPart::Literal(value) => append_all(&mut values, value),
            WordPart::Variable(name) => {
                append_all(&mut values, &host.environment(name).unwrap_or_default())
            }
            WordPart::CommandSubstitution(source) => {
                let bytes = host
                    .command_substitution(source)
                    .map_err(ExpandError::Command)?;
                let value = String::from_utf8_lossy(&bytes)
                    .trim_end_matches(['\r', '\n'])
                    .to_owned();
                append_all(&mut values, &value);
            }
            WordPart::Glob(pattern) => {
                let matches = host.glob(pattern).map_err(ExpandError::Glob)?;
                if matches.is_empty() {
                    append_all(&mut values, pattern);
                } else {
                    values = product(values, matches);
                }
            }
        }
    }
    Ok(values)
}

fn append_all(values: &mut [String], suffix: &str) {
    for value in values {
        value.push_str(suffix);
    }
}

fn product(prefixes: Vec<String>, suffixes: Vec<String>) -> Vec<String> {
    prefixes
        .into_iter()
        .flat_map(|prefix| {
            suffixes.iter().map(move |suffix| {
                let mut value = prefix.clone();
                value.push_str(suffix);
                value
            })
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::BTreeMap;

    #[derive(Default)]
    struct Host(BTreeMap<String, String>);

    impl ExpansionHost for Host {
        fn environment(&self, name: &str) -> Option<String> {
            self.0.get(name).cloned()
        }

        fn command_substitution(&mut self, source: &str) -> Result<Vec<u8>, String> {
            Ok(format!("sub:{source}\n\n").into_bytes())
        }

        fn glob(&self, pattern: &str) -> Result<Vec<String>, String> {
            Ok(if pattern == "*.rs" {
                vec!["a.rs".into(), "b.rs".into()]
            } else {
                Vec::new()
            })
        }
    }

    #[test]
    fn expands_variables_substitutions_and_globs() {
        let mut host = Host::default();
        host.0.insert("NAME".into(), "rat".into());
        let word = Word {
            parts: vec![
                WordPart::Variable("NAME".into()),
                WordPart::Literal(":".into()),
                WordPart::CommandSubstitution("echo hi".into()),
                WordPart::Literal(":".into()),
                WordPart::Glob("*.rs".into()),
            ],
        };
        assert_eq!(
            expand_word(&word, &mut host).unwrap(),
            ["rat:sub:echo hi:a.rs", "rat:sub:echo hi:b.rs"]
        );
    }
}
