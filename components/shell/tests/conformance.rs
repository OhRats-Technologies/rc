use rc_shell::{Connector, RedirectMode, RedirectStream, WordPart, parse};

#[test]
fn bun_style_pipeline_redirect_and_connectors() {
    let script =
        parse("FOO=bar echo \"$FOO world\" | grep world 2>> errors && echo ok || echo no;")
            .unwrap();
    assert_eq!(script.chains.len(), 3);
    assert_eq!(script.chains[0].next, Some(Connector::And));
    assert_eq!(script.chains[1].next, Some(Connector::Or));
    assert_eq!(script.chains[2].next, Some(Connector::Always));
    let pipeline = &script.chains[0].pipeline;
    assert_eq!(pipeline.commands.len(), 2);
    assert_eq!(pipeline.commands[0].assignments[0].name, "FOO");
    assert!(
        pipeline.commands[0].words[1]
            .parts
            .iter()
            .any(|part| matches!(part, WordPart::Variable(name) if name == "FOO"))
    );
    assert_eq!(
        pipeline.commands[1].redirects[0].stream,
        RedirectStream::Stderr
    );
    assert_eq!(pipeline.commands[1].redirects[0].mode, RedirectMode::Append);
}

#[test]
fn quoting_escaping_glob_and_command_substitution_are_distinct() {
    let script =
        parse("echo 'literal $X' \"expanded $X\" escaped\\ space $(echo nested) *.rs").unwrap();
    let words = &script.chains[0].pipeline.commands[0].words;
    assert_eq!(words.len(), 6);
    assert_eq!(words[1].parts, [WordPart::Literal("literal $X".into())]);
    assert!(
        words[2]
            .parts
            .iter()
            .any(|part| matches!(part, WordPart::Variable(_)))
    );
    assert_eq!(words[3].parts, [WordPart::Literal("escaped space".into())]);
    assert!(matches!(
        words[4].parts[0],
        WordPart::CommandSubstitution(_)
    ));
    assert!(matches!(words[5].parts[0], WordPart::Glob(_)));
}

#[test]
fn malformed_source_is_rejected() {
    assert!(parse("echo 'unterminated").is_err());
    assert!(parse("echo hi |").is_err());
    assert!(parse("echo hi >").is_err());
    assert!(parse("$()").is_err());
}

#[test]
fn quoted_empty_words_preserve_empty_arguments() {
    let script = parse("printf '' \"\"").unwrap();
    let words = &script.chains[0].pipeline.commands[0].words;
    assert_eq!(words[1].parts, [WordPart::Literal(String::new())]);
    assert_eq!(words[2].parts, [WordPart::Literal(String::new())]);
}

#[test]
fn command_substitution_balancing_ignores_quoted_parentheses() {
    let script = parse("echo $(echo ')' \"(nested)\")").unwrap();
    assert_eq!(
        script.chains[0].pipeline.commands[0].words[1].parts,
        [WordPart::CommandSubstitution(
            "echo ')' \"(nested)\"".into()
        )]
    );
}
