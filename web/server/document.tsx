import type { ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import type { User } from "../../src/core";
import type { WorkspaceView } from "../../src/workspaces";
import { assetUrl } from "../../src/artifacts";
import { PUBLIC_URL } from "../../src/config";
import { Sidebar } from "./sidebar";

type DocumentProps = {
  title: string; body: ReactNode; user?: User | null; workspaces?: WorkspaceView[]; path?: string;
  scripts?: string[]; styles?: string[]; sidebar?: "open" | "closed"; status?: number; indexable?: boolean;
};

function Document({ title, body, user, workspaces = [], path = "/", scripts = [], styles = [], sidebar = "open", indexable = false }: DocumentProps) {
  const css = assetUrl("styles", "css");
  const socialCard = assetUrl("social-card", "png");
  return <html lang="en" data-sidebar={sidebar}>
    <head>
      <meta charSet="utf-8"/><meta name="viewport" content="width=device-width,initial-scale=1"/>
      <meta name="robots" content={indexable ? "index,follow" : "noindex,nofollow"}/><meta name="color-scheme" content="light dark"/>
      <title>{`${title} | OhRats RC`}</title>
      <meta property="og:site_name" content="OhRats RC"/><meta property="og:type" content="website"/>
      <meta property="og:title" content="OhRats RC | Remote control for your machines"/>
      <meta property="og:description" content="Persistent terminals, saved actions, and private device access without exposing SSH."/>
      <meta property="og:url" content={PUBLIC_URL}/>
      {socialCard ? <><meta property="og:image" content={`${PUBLIC_URL}${socialCard}`}/><meta property="og:image:width" content="2400"/><meta property="og:image:height" content="1260"/><meta property="og:image:type" content="image/png"/><meta property="og:image:alt" content="OhRats RC device page"/></> : null}
      <meta name="twitter:card" content="summary_large_image"/><meta name="twitter:title" content="OhRats RC | Remote control for your machines"/>
      <meta name="twitter:description" content="Persistent terminals, saved actions, and private device access without exposing SSH."/>
      {socialCard ? <><meta name="twitter:image" content={`${PUBLIC_URL}${socialCard}`}/><meta name="twitter:image:alt" content="OhRats RC device page"/></> : null}
      <link rel="icon" type="image/svg+xml" href="https://assets.ohrats.party/assets/logo.092a1cece4d0.svg"/>
      <link rel="stylesheet" href="https://assets.ohrats.party/assets/ohrats.7911fd35d5d3.css"/>
      <link rel="stylesheet" href="https://assets.ohrats.party/assets/states.8d99d4b0e704.css"/>
      <link rel="stylesheet" href="https://assets.ohrats.party/assets/copy.e4c6bbb26b56.css"/>
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
