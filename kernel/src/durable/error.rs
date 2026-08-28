pub(super) enum CommitFailure {
    Conflict(u64),
    Database(rusqlite::Error),
}

impl From<rusqlite::Error> for CommitFailure {
    fn from(error: rusqlite::Error) -> Self {
        Self::Database(error)
    }
}
