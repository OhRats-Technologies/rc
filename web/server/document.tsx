import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import type { User } from "../../src/core";
import type { WorkspaceView } from "../../src/workspaces";
import { assetUrl } from "../../src/artifacts";
import { Sidebar } from "./sidebar";

type DocumentProps = {
  title: string; body: ReactNode; user?: User | null; workspaces?: WorkspaceView[]; path?: string;
  scripts?: string[]; styles?: string[]; sidebar?: "open" | "closed"; status?: number;
};

function Document({ title, body, user, workspaces = [], path = "/", scripts = [], styles = [], sidebar = "open" }: DocumentProps) {
  const css = assetUrl("styles", "css");
  return <html lang="en" data-sidebar={sidebar}>
    <head>
      <meta charSet="utf-8"/><meta name="viewport" content="width=device-width,initial-scale=1"/>
      <meta name="robots" content="noindex,nofollow"/><meta name="color-scheme" content="light dark"/>
      <title>{`${title} | Relay`}</title>
      <link rel="icon" type="image/svg+xml" href="https://assets.ohrats.party/assets/logo.092a1cece4d0.svg"/>
      <link rel="stylesheet" href="https://assets.ohrats.party/assets/ohrats.7911fd35d5d3.css"/>
      {css && <link rel="stylesheet" href={css}/>} {styles.map(name => assetUrl(name, "css")).filter(Boolean).map(src => <link key={src} rel="stylesheet" href={src}/>)}<script src="https://assets.ohrats.party/assets/theme.b6e0fe408633.js"/>
    </head>
    <body className={user ? "authenticated" : undefined}>
      {user ? <div className="site-shell"><Sidebar user={user} workspaces={workspaces} path={path}/><main className="site-content">{body}</main></div> : body}
      {user && assetUrl("sidebar") && (
        <script type="module" src={assetUrl("sidebar")}/>
      )}
      {scripts.map(name => assetUrl(name)).filter(Boolean).map(src => <script key={src} type="module" src={src}/>) }
    </body>
  </html>;
}

export function htmlDocument(props: DocumentProps, authenticated = !!props.user) {
  return new Response(`<!doctype html>${renderToStaticMarkup(<Document {...props}/>)}`, {
    status: props.status || 200,
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": authenticated ? "no-store" : "public, max-age=0, must-revalidate",
      ...(authenticated ? { "cloudflare-cdn-cache-control": "no-store" } : {}),
    },
  });
}
