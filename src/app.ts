import { Elysia } from "elysia";

export const app = new Elysia({ name: "relay" });

export type App = typeof app;
