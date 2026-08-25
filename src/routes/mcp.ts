import { Elysia } from "elysia";
import { handleMcp } from "../mcp/server";

export const mcpRoutes = new Elysia({ name: "rc.mcp", detail: { hide: true } })
  .all("/mcp", ({ request }) => handleMcp(request));
