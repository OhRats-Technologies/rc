import { join } from "node:path";

export const PORT = Number(process.env.PORT || 3000);
export const VERSION = "0.2.1";
export const DATA_DIR = process.env.DATA_DIR || "./data";
export const PUBLIC_URL = (process.env.PUBLIC_URL || `http://localhost:${PORT}`).replace(/\/$/, "");
export const SETUP_TOKEN = String(process.env.RELAY_SETUP_TOKEN || "").trim();
export const RP_ID = new URL(PUBLIC_URL).hostname;
export const DB_PATH = join(DATA_DIR, "relay.db");
export const SESSION_TTL = 30 * 24 * 60 * 60 * 1000;
export const TOKEN_TTL = 24 * 60 * 60 * 1000;
export const SETUP_COOKIE_TTL = 15 * 60;
export const CEREMONY_TTL = 5 * 60 * 1000;
