import { Elysia } from "elysia";
import { frontendAsset, installScript } from "../artifacts";

export const artifactRoutes = new Elysia({ name: "rc.artifacts", detail: { hide: true } })
  .get("/install.sh", installScript)
  .get("/assets/*", ({ params }) => frontendAsset(params["*"]));
