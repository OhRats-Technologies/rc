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
  description?: string; canonicalPath?: string; publicSite?: boolean;
};

function Document({ title, body, user, workspaces = [], path = "/", scripts = [], styles = [], sidebar = "open", indexable = false,
  description = "Persistent terminals, saved actions, and private device access without exposing SSH.", canonicalPath = "/", publicSite = false }: DocumentProps) {
  const css = assetUrl("styles", "css");
  const socialCard = assetUrl("social-card", "png");
  const canonical = `${PUBLIC_URL}${canonicalPath}`;
  return <html lang="en" data-sidebar={sidebar}>
    <head>
      <meta charSet="utf-8"/><meta name="viewport" content="width=device-width,initial-scale=1"/>
      <meta name="robots" content={indexable ? "index,follow" : "noindex,nofollow"}/><meta name="color-scheme" content="light dark"/>
      <title>{`${title} | OhRats RC`}</title>
      {indexable && <link rel="canonical" href={canonical}/>}<meta name="description" content={description}/>
      <meta property="og:site_name" content="OhRats RC"/><meta property="og:type" content="website"/>
      <meta property="og:title" content={`${title} | OhRats RC`}/><meta property="og:description" content={description}/><meta property="og:url" content={canonical}/>
      {socialCard ? <><meta property="og:image" content={`${PUBLIC_URL}${socialCard}`}/><meta property="og:image:width" content="2400"/><meta property="og:image:height" content="1260"/><meta property="og:image:type" content="image/png"/><meta property="og:image:alt" content="OhRats RC device page"/></> : null}
      <meta name="twitter:card" content="summary_large_image"/><meta name="twitter:title" content={`${title} | OhRats RC`}/><meta name="twitter:description" content={description}/>
      {socialCard ? <><meta name="twitter:image" content={`${PUBLIC_URL}${socialCard}`}/><meta name="twitter:image:alt" content="OhRats RC device page"/></> : null}
      <link rel="icon" type="image/svg+xml" href="https://assets.ohrats.party/assets/logo.092a1cece4d0.svg"/>
      <link rel="stylesheet" href="https://assets.ohrats.party/assets/ohrats.bb9b49c66e85.css"/>
      <link rel="stylesheet" href="https://assets.ohrats.party/assets/states.8d99d4b0e704.css"/>
      <link rel="stylesheet" href="https://assets.ohrats.party/assets/copy.e4c6bbb26b56.css"/>
      <link rel="preconnect" href="https://fonts.googleapis.com"/><link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous"/>
      <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Space+Mono:wght@400;700&display=swap"/>
      {css && <link rel="stylesheet" href={css}/>} {styles.map(name => assetUrl(name, "css")).filter(Boolean).map(src => <link key={src} rel="stylesheet" href={src}/>)}<script src="https://assets.ohrats.party/assets/theme.b6e0fe408633.js"/>
    </head>
    <body className={user ? "authenticated" : undefined}>
      {user ? <div className="site-shell"><Sidebar user={user} workspaces={workspaces} path={path}/><main className="site-content">{body}</main></div> : body}
      {user && assetUrl("sidebar") && (
        <script type="module" src={assetUrl("sidebar")}/>
      )}
      {publicSite && <script src="https://assets.ohrats.party/assets/menu.a8b9a29f9ccc.js" defer/>}
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
