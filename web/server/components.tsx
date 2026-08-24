export function SectionBadge({ index, children }: { index: string; children: string }) {
  return <div className="badge-container">
    <span className="badge-text">{index} {children}</span>
    <div className="badge-line"/>
  </div>;
}
