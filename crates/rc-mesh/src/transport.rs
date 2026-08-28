use crate::MeshEnvelope;
use async_trait::async_trait;
use bytes::Bytes;

#[derive(Debug, Clone, thiserror::Error, PartialEq, Eq)]
pub enum RouteError {
    #[error("no mesh route is available")]
    Unavailable,
    #[error("mesh route rejected the envelope")]
    Rejected,
    #[error("mesh route disconnected")]
    Disconnected,
}

#[async_trait]
pub trait RouteProvider: Send + Sync {
    async fn send(&self, envelope: &MeshEnvelope) -> Result<(), RouteError>;
}

#[derive(Debug, Clone, thiserror::Error, PartialEq, Eq)]
pub enum FrameTransportError {
    #[error("encrypted transport is closed")]
    Closed,
    #[error("encrypted transport rejected the frame")]
    Rejected,
}

#[async_trait]
pub trait EncryptedFrameTransport: Send + Sync {
    async fn send(&self, frame: Bytes) -> Result<(), FrameTransportError>;
}
