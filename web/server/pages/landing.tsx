import { htmlDocument } from "../document";
import { SectionBadge } from "../components";
import { Arrow, PublicFooter, PublicNav } from "../public";
import { PUBLIC_SIGNUP_CONFIGURED } from "../../../src/config";

export function landingPage() {
  return htmlDocument({
    title: "Remote Control", description: "Private remote control for your machines without exposing SSH.",
    canonicalPath: "/", styles: ["public"], indexable: true, publicSite: true, body:
    <div className="public-site">
      <PublicNav/>
      <main>
        <header className="hero-header">
          <div className="ohrats-grid"/>
          <div className="hero-content">
            <h1 aria-label="Remote Control for your machines.">Remote Control<br/><span className="hero-muted">for your machines.</span></h1>
            <div className="public-hero-actions"><a href={PUBLIC_SIGNUP_CONFIGURED ? "/signup" : "/docs"} className="or-button">Get started <Arrow/></a><a href="/login" className="header-link">Sign in <Arrow/></a></div>
          </div>
        </header>

        <section className="public-section">
          <div className="container">
            <div className="section-grid">
              <div className="section-heading-stack"><SectionBadge index="01">Security</SectionBadge><div className="section-title-container"><h2>Security model</h2></div></div>
              <div className="public-safety-list">
                <div className="public-safety-item"><h3>Passkeys</h3><p>Human sign-in and sensitive approvals use WebAuthn. RC does not use account passwords.</p></div>
                <div className="public-safety-item"><h3>RC Lock</h3><p>Each Node stores workspace authority locally and verifies Owner-signed changes before execution.</p></div>
                <div className="public-safety-item"><h3>Encrypted control</h3><p>Browser and CLI process traffic is encrypted client-to-Node; hosted process history keeps metadata only.</p></div>
                <div className="public-safety-item"><h3>Scoped automation</h3><p>API requests are signed, MCP grants select machines and capabilities, and Node updates require signed releases.</p></div>
              </div>
            </div>
          </div>
        </section>

        <section className="public-section">
          <div className="container">
            <header className="section-header public-resources-heading">
              <div className="section-heading-stack"><SectionBadge index="02">Resources</SectionBadge><div className="section-title-container"><h2>Documentation</h2></div></div>
              <div className="public-resource-links"><a className="header-link" href="/docs">Docs <Arrow/></a></div>
            </header>
          </div>
        </section>
      </main>
      <PublicFooter/>
    </div>,
  });
}
