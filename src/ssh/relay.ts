import type { AgentServerMessage } from "../protocol";

type Sender = (deviceId: string, message: AgentServerMessage) => boolean;
let sender: Sender = () => false;

export function registerSshSender(value: Sender) { sender = value; }
export function sendSshAgent(deviceId: string, message: AgentServerMessage) { return sender(deviceId, message); }
