use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ProcessEvent {
    Started {
        id: String,
    },
    Stdout {
        id: String,
        data: String,
    },
    Stderr {
        id: String,
        data: String,
    },
    Exit {
        id: String,
        exit_code: i32,
        signal: String,
    },
}

impl ProcessEvent {
    pub fn output(kind: StreamKind, id: &str, bytes: &[u8]) -> Self {
        let data = URL_SAFE_NO_PAD.encode(bytes);
        match kind {
            StreamKind::Stdout => Self::Stdout {
                id: id.into(),
                data,
            },
            StreamKind::Stderr => Self::Stderr {
                id: id.into(),
                data,
            },
        }
    }

    pub fn id(&self) -> &str {
        match self {
            Self::Started { id }
            | Self::Stdout { id, .. }
            | Self::Stderr { id, .. }
            | Self::Exit { id, .. } => id,
        }
    }

    pub fn estimated_size(&self) -> usize {
        match self {
            Self::Stdout { data, .. } | Self::Stderr { data, .. } => data.len() + 128,
            _ => 128,
        }
    }

    pub fn is_output(&self) -> bool {
        matches!(self, Self::Stdout { .. } | Self::Stderr { .. })
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum StreamKind {
    Stdout,
    Stderr,
}
