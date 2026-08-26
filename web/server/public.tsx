import { PUBLIC_SIGNUP_CONFIGURED } from "../../src/config";

const logo = "https://assets.ohrats.party/assets/logo.092a1cece4d0.svg";

function Arrow() {
  return <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <path d="M5 12h14M12 5l7 7-7 7"/>
  </svg>;
}

const resources = [
  ["01", "Docs", "/docs"],
] as const;

export function PublicNav({ active = "", documentation = false }: { active?: string; documentation?: boolean }) {
  return <nav className="site-nav">
    <div className="nav-container">
      <div className="nav-left">
        <a href="/" className="logo" aria-label="RC home"><img src={logo} alt="OhRats" className="logo-image"/><span className="logo-text">RC</span></a>
        <div className="desktop-menu">{resources.map(([number, label, href]) =>
          <a href={href} className={`menu-item${active === label.toLowerCase() ? " nav-highlight" : ""}`} key={href}>
            <span className="menu-number">{number}</span><span className="menu-label">{label}</span>
          </a>)}</div>
      </div>
      <div className="cta-buttons">
        <button className="theme-toggle" data-theme-toggle aria-label="Toggle theme"/>
        <a href="/login" className="cta-link">Sign in</a>
        {!documentation && <a href={PUBLIC_SIGNUP_CONFIGURED ? "/signup" : "/docs"} className="or-button">Get started <Arrow/></a>}
      </div>
      <button className="mobile-menu-btn" data-menu-toggle aria-controls="mobile-menu" aria-expanded="false" aria-label="Open menu">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/></svg>
      </button>
    </div>
    <div className="mobile-menu" id="mobile-menu">
      {resources.map(([number, label, href]) => <a href={href} className="mobile-menu-link" key={href}>{number} {label}</a>)}
      <div className="mobile-divider"/>
      <div className="mobile-theme-row"><span className="mobile-theme-label">Theme</span><button className="mobile-theme-btn" data-theme-toggle aria-label="Toggle theme"/></div>
      <a href="/login" className="mobile-menu-link">Sign in</a>
      {PUBLIC_SIGNUP_CONFIGURED && <a href="/signup" className="mobile-menu-link">Create account</a>}
    </div>
  </nav>;
}

export function PublicFooter() {
  return <footer>
    <div className="footer-content">
      <div className="footer-brand">
        <a href="https://ohrats.party/" aria-label="OhRats Technologies home"><img src={logo} alt="OhRats" className="footer-logo"/></a>
        <div className="footer-socials">
          <a href="https://x.com/ohratsparty" className="footer-social-link" aria-label="X (Twitter)"><svg viewBox="0 0 32 32" fill="none"><path d="M28 28L18.6145 14.0124L18.6305 14.0255L27.0929 4H24.265L17.3713 12.16L11.8968 4H4.48021L13.2425 17.0593L13.2414 17.0582L4 28H6.82792L14.4921 18.9215L20.5834 28H28ZM10.7763 6.18182L23.9449 25.8182H21.7039L8.52468 6.18182H10.7763Z" fill="currentColor"/></svg></a>
          <a href="https://github.com/OhRats-Technologies" className="footer-social-link" aria-label="GitHub"><span className="github-icon" aria-hidden="true"/></a>
        </div>
      </div>
      <nav className="footer-links">
        <div className="footer-column"><span className="footer-column-label">Tools</span><a href="https://wakebar.ohrats.party/" className="footer-link">WakeBar</a><a href="https://rc.ohrats.party/" className="footer-link">RC</a></div>
        <div className="footer-column"><span className="footer-column-label">Resources</span><a href="https://ohrats.party/blog" className="footer-link">Blog</a><a href="https://github.com/OhRats-Technologies" className="footer-link">GitHub</a></div>
        <div className="footer-column"><span className="footer-column-label">Company</span><a href="https://ohrats.party/#research" className="footer-link">Research</a><a href="mailto:contact@ohrats.party" className="footer-link">Contact</a></div>
      </nav>
    </div>
    <div className="footer-bottom"><span className="footer-copyright">© 2026 OhRats Technologies. All rights reserved.</span></div>
  </footer>;
}

export { Arrow };
