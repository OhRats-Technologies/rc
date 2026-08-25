import { AUTH_LIFETIME_OPTIONS, type AuthLifetime } from "../../src/lifetimes";

export function SectionBadge({ index, children }: { index: string; children: string }) {
  return <div className="badge-container">
    <span className="badge-text">{index} {children}</span>
    <div className="badge-line"/>
  </div>;
}

export function LifetimeSelect({ defaultValue, allowNever = true, name = "lifetime", label = "Access duration" }: {
  defaultValue: AuthLifetime; allowNever?: boolean; name?: string; label?: string;
}) {
  return <label>{label}<select name={name} defaultValue={defaultValue}>
    {AUTH_LIFETIME_OPTIONS.filter(option => allowNever || option.value !== "never").map(option =>
      <option value={option.value} key={option.value}>{option.label}</option>)}
  </select></label>;
}
