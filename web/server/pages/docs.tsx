import { htmlDocument } from "../document";
import { PublicFooter, PublicNav } from "../public";
import { docGroups, docHref } from "../docs/catalog";
import type { DocArticle } from "../docs/types";

function Catalog({ active }: { active: string }) {
  return <nav className="docs-catalog" aria-label="Documentation">
    {docGroups.map(group => <div className="docs-catalog-group" key={group.title}>
      <span className="docs-catalog-label">{group.title}</span>
      <div className="docs-catalog-links">{group.items.map(item =>
        <a href={item.href} className={`docs-catalog-link${item.slug === active ? " active" : ""}`} aria-current={item.slug === active ? "page" : undefined} key={item.slug}>{item.title}</a>)}</div>
    </div>)}
  </nav>;
}

function OnThisPage({ article }: { article: DocArticle }) {
  return <nav className="docs-toc-nav" aria-label="On this page">
    <span className="docs-toc-label">On this page</span>
    <div className="docs-toc-links">{article.sections.map(section =>
      <a href={`#${section.id}`} key={section.id}>{section.title}</a>)}</div>
  </nav>;
}

export function docsPage(article: DocArticle) {
  return htmlDocument({
    title: article.title,
    description: article.intro,
    canonicalPath: docHref(article.slug),
    styles: ["public"],
    scripts: article.copy ? ["copy"] : [],
    indexable: true,
    publicSite: true,
    body: <div className="public-site docs-site">
      <PublicNav active="docs" documentation/>
      <main className="docs-layout">
        <aside className="docs-sidebar"><Catalog active={article.slug}/></aside>
        <article className="docs-article">
          <details className="docs-mobile-catalog">
            <summary>Documentation</summary>
            <Catalog active={article.slug}/>
          </details>
          <header className="docs-article-header">
            <h1>{article.title}</h1>
            <p>{article.intro}</p>
          </header>
          <div className="docs-content">{article.sections.map(section =>
            <section className="docs-article-section" id={section.id} key={section.id}>
              <h2>{section.title}</h2>
              <div className="docs-copy">{section.body}</div>
            </section>)}</div>
        </article>
        <aside className="docs-toc"><OnThisPage article={article}/></aside>
      </main>
      <PublicFooter/>
    </div>,
  });
}
