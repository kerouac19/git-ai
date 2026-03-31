function parseBooleanFlag(value?: string) {
  if (!value) {
    return false;
  }

  return ['1', 'true', 'yes', 'on'].includes(value.trim().toLowerCase());
}

export function resolvePostgresDatabaseUrl() {
  const databaseUrl = process.env.DATABASE_URL?.trim();

  if (databaseUrl) {
    return databaseUrl;
  }

  const host = process.env.DB_HOST || process.env.PGHOST || '127.0.0.1';
  const port = Number(process.env.DB_PORT || process.env.PGPORT || 5432);
  const username = process.env.DB_USER || process.env.PGUSER || process.env.USER || 'postgres';
  const password = process.env.DB_PASSWORD || process.env.PGPASSWORD || '';
  const database = process.env.DB_NAME || process.env.PGDATABASE || 'git_ai';
  const sslEnabled = parseBooleanFlag(process.env.DB_SSL);

  const credentials = password
    ? `${encodeURIComponent(username)}:${encodeURIComponent(password)}`
    : encodeURIComponent(username);
  const searchParams = new URLSearchParams();

  if (sslEnabled) {
    const rejectUnauthorized = parseBooleanFlag(process.env.DB_SSL_REJECT_UNAUTHORIZED);
    searchParams.set('sslmode', rejectUnauthorized ? 'verify-full' : 'require');
  }

  const query = searchParams.toString();
  return `postgresql://${credentials}@${host}:${port}/${database}${query ? `?${query}` : ''}`;
}

export function describeDatabaseTarget() {
  if (process.env.DATABASE_URL?.trim()) {
    return 'postgres:DATABASE_URL';
  }

  const host = process.env.DB_HOST || process.env.PGHOST || '127.0.0.1';
  const port = process.env.DB_PORT || process.env.PGPORT || '5432';
  const database = process.env.DB_NAME || process.env.PGDATABASE || 'git_ai';
  return `postgres://${host}:${port}/${database}`;
}
