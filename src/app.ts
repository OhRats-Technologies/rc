import { Elysia } from "elysia";
import { apiRoutes } from "./routes/api";

export const app = new Elysia({ name: "relay" }).use(apiRoutes);

export type App = typeof app;
