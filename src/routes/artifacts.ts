import { Elysia } from "elysia";
import { download, frontendAsset, installScript } from "../artifacts";

export const artifactRoutes = new Elysia({ name: "rc.artifacts", detail: { hide: true } })
  .get("/install.sh", installScript)
  .get("/downloads/*", ({ params }) => download(params["*"]))
  .get("/assets/*", ({ params }) => frontendAsset(params["*"]));
