import type { AgentServerMessage } from "../protocol";

type Sender = (deviceId: string, message: AgentServerMessage) => boolean;
let sender: Sender = () => false;

export function registerMcpSender(value: Sender) { sender = value; }
export function sendMcpAgent(deviceId: string, message: AgentServerMessage) { return sender(deviceId, message); }
