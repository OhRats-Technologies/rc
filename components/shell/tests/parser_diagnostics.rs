use rc_shell::parse;

#[test]
fn unsupported_heredoc_has_explicit_diagnostic() {
    let source = "python3 - <<'PY'\nprint(\"RC_HEREDOC_PROBE\")\nPY";
    let error = parse(source).unwrap_err();
    assert_eq!(error.offset, 10);
    assert_eq!(
        error.message,
        "heredocs (<<) are unsupported in portable RC Shell; use argv to explicitly select a shell that supports them (for example bash)"
    );
}

#[test]
fn syntax_errors_use_utf8_byte_offsets_including_eof() {
    for (source, offset) in [
        ("echo café > | cat", 13),
        ("echo café >   ", 15),
        ("echo hello |", 12),
    ] {
        let error = parse(source).unwrap_err();
        assert_eq!(error.offset, offset, "{source}: {error}");
        assert!(error.to_string().ends_with(&format!("at byte {offset}")));
    }
}

#[test]
fn quoted_and_escaped_heredoc_markers_remain_literal() {
    for source in ["echo '<<'", "echo \"<<\"", r"echo \<\<", "cat < input"] {
        assert!(parse(source).is_ok(), "{source}");
    }
}
