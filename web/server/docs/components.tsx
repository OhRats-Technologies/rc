import type { ReactNode } from "react";

export function CopyField({ value, prefix = "$" }: { value: string; prefix?: string }) {
  return <div className="or-copy-field" data-copy-value={value} title={value}>
    {prefix && <span className="or-copy-prefix">{prefix}</span>}<code>{value}</code>
    <button className="or-copy-button copy-value" type="button" aria-label="Copy"><span className="or-copy-icon" aria-hidden="true"/></button>
  </div>;
}

export function CodeBlock({ children }: { children: string }) {
  return <pre className="docs-code"><code>{children}</code></pre>;
}

export function DocTable({ children }: { children: ReactNode }) {
  return <div className="docs-table-wrap"><table className="docs-table">{children}</table></div>;
}
