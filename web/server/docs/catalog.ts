export const docGroups = [
  {
    title: "Overview",
    items: [
      { slug: "quickstart", title: "Quickstart", href: "/docs" },
      { slug: "principles", title: "Principles", href: "/docs/principles" },
      { slug: "security", title: "Security model", href: "/docs/security" },
      { slug: "authentication", title: "Authentication", href: "/docs/authentication" },
    ],
  },
  {
    title: "Interfaces",
    items: [
      { slug: "cli", title: "CLI", href: "/docs/cli" },
      { slug: "mcp", title: "MCP", href: "/docs/mcp" },
      { slug: "api", title: "API", href: "/docs/api" },
    ],
  },
] as const;

export function docHref(slug: string) {
  for (const group of docGroups) {
    const item = group.items.find(value => value.slug === slug);
    if (item) return item.href;
  }
  return "/docs";
}
