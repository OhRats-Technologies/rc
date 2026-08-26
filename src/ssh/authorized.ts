export {};

const [algorithm = "", key = ""] = process.argv.slice(2);
if (!algorithm || !key) process.exit(1);
const port = process.env.RC_SSH_INTERNAL_PORT || "3001";
try {
  const response = await fetch(`http://127.0.0.1:${port}/authorized?type=${encodeURIComponent(algorithm)}&key=${encodeURIComponent(key)}`);
  if (!response.ok) process.exit(1);
  process.stdout.write(await response.text());
} catch {
  process.exit(1);
}
