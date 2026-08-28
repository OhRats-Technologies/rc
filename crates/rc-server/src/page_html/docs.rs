use super::public_snapshots::{self, PublicPage};

pub fn docs(topic: &str, public_url: &str, public_signup: bool) -> Option<String> {
    let page = match topic {
        "quickstart" => PublicPage::Quickstart,
        "principles" => PublicPage::Principles,
        "security" => PublicPage::Security,
        "authentication" => PublicPage::Authentication,
        "cli" => PublicPage::Cli,
        "mcp" => PublicPage::Mcp,
        "api" => PublicPage::Api,
        _ => return None,
    };
    Some(public_snapshots::render(page, public_signup, public_url))
}
