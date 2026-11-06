package protocol

const (
	ProtocolVersion         uint32 = 1
	NodeControlMessageLimit int    = 1048576         // 1 MiB
	ProcessInputChunkLimit  int    = 128 * 1024      // 128 KiB
	McpImageChunkBytes      int    = 512 * 1024      // 512 KiB
	McpImageMaxBytes        int    = 8 * 1024 * 1024 // 8 MiB
	WireLimit               int    = 48 * 1024       // 48 KiB
	FragmentBytes           int    = 8 * 1024        // 8 KiB
)
