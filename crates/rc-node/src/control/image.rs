use super::ControlManager;
use crate::{hosted_control_authority, verify_mcp_grant};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_protocol::{ControlProof, MCP_IMAGE_CHUNK_BYTES, MCP_IMAGE_MAX_BYTES, NodeToServer};
use std::path::Path;

impl ControlManager {
    #[allow(clippy::too_many_arguments)]
    pub(super) async fn mcp_image_view(
        &self,
        request_id: String,
        user_id: String,
        path: String,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    ) {
        if request_id.is_empty() || path.trim().is_empty() {
            self.image_error(request_id, "invalid image request");
            return;
        }
        let proof = ControlProof {
            grant: control_grant,
            credential_id,
            assertion: control_assertion,
        };
        let Ok(authority) = hosted_control_authority(&self.0.state_dir, &proof, &user_id) else {
            self.image_error(request_id, "image access is not authorized");
            return;
        };
        if verify_mcp_grant(
            &self.0.state_dir,
            &mcp_grant,
            &mcp_signature,
            &authority,
            &user_id,
            &self.0.state.device_id,
        )
        .is_err()
        {
            self.image_error(request_id, "image access is not authorized");
            return;
        }
        let Some(mime_type) = image_mime_type(&path) else {
            self.image_error(request_id, "unsupported image type");
            return;
        };
        let metadata = match tokio::fs::metadata(&path).await {
            Ok(value) if value.is_file() => value,
            Ok(_) => {
                self.image_error(request_id, "image path is not a file");
                return;
            }
            Err(error) => {
                self.image_error(request_id, format!("failed to read image: {error}"));
                return;
            }
        };
        if metadata.len() > MCP_IMAGE_MAX_BYTES as u64 {
            self.image_error(
                request_id,
                format!("image exceeds the {MCP_IMAGE_MAX_BYTES}-byte limit"),
            );
            return;
        }
        let bytes = match tokio::fs::read(&path).await {
            Ok(value) => value,
            Err(error) => {
                self.image_error(request_id, format!("failed to read image: {error}"));
                return;
            }
        };
        for chunk in bytes.chunks(MCP_IMAGE_CHUNK_BYTES) {
            self.emit(NodeToServer::McpImageChunk {
                request_id: request_id.clone(),
                data: URL_SAFE_NO_PAD.encode(chunk),
            });
        }
        self.emit(NodeToServer::McpImageResult {
            request_id,
            mime_type: mime_type.into(),
            size_bytes: bytes.len() as u64,
            error: String::new(),
        });
    }

    fn image_error(&self, request_id: String, error: impl Into<String>) {
        self.emit(NodeToServer::McpImageResult {
            request_id,
            mime_type: String::new(),
            size_bytes: 0,
            error: error.into(),
        });
    }
}

fn image_mime_type(path: &str) -> Option<&'static str> {
    let extension = Path::new(path).extension()?.to_str()?.to_ascii_lowercase();
    match extension.as_str() {
        "png" => Some("image/png"),
        "jpg" | "jpeg" => Some("image/jpeg"),
        "webp" => Some("image/webp"),
        "gif" => Some("image/gif"),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::image_mime_type;

    #[test]
    fn image_mime_type_is_small_and_explicit() {
        assert_eq!(image_mime_type("shot.PNG"), Some("image/png"));
        assert_eq!(image_mime_type("photo.jpeg"), Some("image/jpeg"));
        assert_eq!(image_mime_type("frame.webp"), Some("image/webp"));
        assert_eq!(image_mime_type("anim.gif"), Some("image/gif"));
        assert_eq!(image_mime_type("notes.txt"), None);
    }
}
