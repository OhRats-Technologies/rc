import { CopyField } from "./components";
import type { DocArticle } from "./types";
import { PUBLIC_SIGNUP_CONFIGURED } from "../../../src/config";

export function quickstartArticle(): DocArticle {
  return {
    slug: "quickstart",
    title: "Quickstart",
    intro: "Create an account, enroll a machine, and open a remote shell.",
    copy: true,
    sections: [
      {
        id: "requirements",
        title: "Requirements",
        body: <>
          <ul>
            <li>{PUBLIC_SIGNUP_CONFIGURED ? <>An RC account. The OhRats-managed service supports public passkey signup.</> : <>An existing RC account or a workspace invitation.</>}</li>
            <li>A passkey-capable browser for account creation and sensitive approvals.</li>
            <li>A macOS or Linux machine to enroll.</li>
          </ul>
        </>,
      },
      {
        id: "create-account",
        title: "Create an account",
        body: <>
          <ol>
            <li>{PUBLIC_SIGNUP_CONFIGURED ? <>Open <a href="/signup">Create account</a>.</> : <>Open the workspace invitation provided by an Owner.</>}</li>
            <li>Choose your RC user name.</li>
            <li>Create a passkey when the browser asks.</li>
          </ol>
          <p>The passkey is the account credential. RC does not use account passwords. Every new account receives its own <strong>Personal</strong> workspace; invitation signup also joins the invited workspace.</p>
        </>,
      },
      {
        id: "enroll-machine",
        title: "Enroll a machine",
        body: <>
          <ol>
            <li>Sign in and open <strong>Devices</strong>.</li>
            <li>Choose <strong>Enroll device</strong> and select a workspace you own.</li>
            <li>Choose <strong>Create install command</strong>.</li>
            <li>Run the generated command on the machine you want to control.</li>
          </ol>
          <p>The generated command contains a one-time enrollment token. It downloads and checksum-verifies the current GitHub Release, enrolls the machine, and installs a per-user background service.</p>
          <p>The service is a LaunchAgent on macOS and a user systemd service on supported Linux systems.</p>
        </>,
      },
      {
        id: "verify-node",
        title: "Verify the Node",
        body: <>
          <CopyField value="rc status"/>
          <p>The command shows local enrollment and the hosted device state. The Devices page should show the same machine as online.</p>
        </>,
      },
      {
        id: "remote-shell",
        title: "Open a remote shell",
        body: <>
          <p>You can open a terminal from the device page in the browser, or use the CLI:</p>
          <CopyField value="rc login"/>
          <CopyField value="rc devices"/>
          <CopyField value="rc shell DEVICE"/>
          <p><code>login</code> opens RC for passkey approval. The resulting CLI authorization defaults to until revoked.</p>
          <p>For standard OpenSSH tooling, register a public key with <code>rc ssh-key add</code> and add the output of <code>rc ssh-config</code> to your SSH config.</p>
        </>,
      },
      {
        id: "next",
        title: "Next",
        body: <ul>
          <li><a href="/docs/security">Security model</a> explains RC Lock and encrypted control.</li>
          <li><a href="/docs/authentication">Authentication</a> covers passkeys, CLI sessions, API keys, and MCP OAuth.</li>
          <li><a href="/docs/cli">CLI</a>, <a href="/docs/mcp">MCP</a>, and <a href="/docs/api">API</a> contain interface-specific reference material.</li>
        </ul>,
      },
    ],
  };
}
