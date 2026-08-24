import { Elysia } from "elysia";
import { handleAPI } from "../router";

export const apiRoutes = new Elysia({ name: "relay.api" })
  .all("/api/v1/*", ({ request }) => handleAPI(request, new URL(request.url)));
