use super::public_snapshots::{self, PublicPage};

pub fn landing(public_url: &str, public_signup: bool) -> String {
    public_snapshots::render(PublicPage::Landing, public_signup, public_url)
}
