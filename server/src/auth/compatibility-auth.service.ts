import { Injectable, UnauthorizedException } from '@nestjs/common';
import { JwtService } from '@nestjs/jwt';
import * as crypto from 'crypto';
import { PrismaService } from '../prisma/prisma.service';

type CompatRole = 'admin' | 'user' | 'moderator' | 'guest';
type DeviceCodeStatus = 'pending' | 'approved' | 'denied';

interface DeviceCodeEntry {
  deviceCode: string;
  userCode: string;
  createdAt: number;
  expiresAt: number;
  status: DeviceCodeStatus;
  approvedAt?: number;
  deniedAt?: number;
  lastPolledAt?: number;
  subject: TokenSubject;
}

interface TokenSubject {
  sub: string;
  email: string;
  name: string;
  personal_org_id: string;
  orgs: Array<{
    org_id: string;
    org_name: string;
    org_slug: string;
    role: CompatRole;
  }>;
  role: CompatRole;
}

interface DeviceCodeRecord {
  device_code: string;
  user_code: string;
  status: DeviceCodeStatus;
  expires_at: Date;
  approved_at: Date | null;
  denied_at: Date | null;
  last_polled_at: Date | null;
  subject_json: string;
}

@Injectable()
export class CompatibilityAuthService {
  private readonly accessTokenTtlSeconds = 60 * 60;
  private readonly refreshTokenTtlSeconds = 60 * 60 * 24 * 90;
  private readonly deviceCodePollIntervalSeconds = 5;

  constructor(
    private readonly jwtService: JwtService,
    private readonly prisma: PrismaService,
  ) {}

  async startDeviceFlow(baseUrl: string) {
    await this.pruneExpiredDeviceCodes();

    const now = new Date();
    const expiresIn = 15 * 60;
    const deviceCode = crypto.randomUUID();
    const userCode = this.makeUserCode();
    const verificationUri = `${baseUrl}/oauth/device`;
    const subject = this.makeDefaultSubject();
    const entry: DeviceCodeEntry = {
      deviceCode,
      userCode,
      createdAt: now.getTime(),
      expiresAt: now.getTime() + expiresIn * 1000,
      status: 'pending',
      subject,
    };

    await this.prisma.$executeRaw`
      insert into public.oauth_device_codes (
        device_code,
        user_code,
        client_id,
        verification_uri,
        status,
        user_id,
        subject_json,
        created_at,
        expires_at
      )
      values (
        ${entry.deviceCode},
        ${entry.userCode},
        ${'git-ai-cli'},
        ${verificationUri},
        ${entry.status},
        ${subject.sub},
        ${JSON.stringify(subject)}::jsonb,
        ${now},
        ${new Date(entry.expiresAt)}
      )
    `;

    return {
      device_code: deviceCode,
      user_code: userCode,
      verification_uri: verificationUri,
      verification_uri_complete: `${verificationUri}?user_code=${encodeURIComponent(userCode)}`,
      expires_in: expiresIn,
      interval: this.deviceCodePollIntervalSeconds,
    };
  }

  async exchangeDeviceCode(deviceCode: string) {
    await this.pruneExpiredDeviceCodes();

    const entry = await this.findDeviceCodeByDeviceCode(deviceCode);
    if (!entry) {
      return this.oauthError('expired_token', 'Device code expired or not found');
    }

    const now = new Date();
    if (entry.status === 'denied') {
      return this.oauthError('access_denied', 'Device authorization was denied');
    }

    if (entry.status === 'approved') {
      await this.deleteDeviceCode(deviceCode);
      return this.issueTokenResponse(entry.subject);
    }

    if (
      entry.lastPolledAt &&
      now.getTime() - entry.lastPolledAt.getTime() < this.deviceCodePollIntervalSeconds * 1000
    ) {
      await this.touchDeviceCodePoll(deviceCode, now);
      return this.oauthError('slow_down', 'Polling too frequently');
    }

    await this.touchDeviceCodePoll(deviceCode, now);
    return this.oauthError('authorization_pending', 'Device authorization is still pending');
  }

  exchangeRefreshToken(refreshToken: string) {
    try {
      const payload = this.jwtService.verify(refreshToken) as Record<string, unknown>;
      if (payload.type !== 'refresh') {
        return this.oauthError('invalid_grant', 'Refresh token is invalid');
      }

      return this.issueTokenResponse(this.subjectFromPayload(payload));
    } catch (_error) {
      return this.oauthError('invalid_grant', 'Refresh token is invalid or expired');
    }
  }

  exchangeInstallNonce(installNonce: string) {
    if (!installNonce || !installNonce.trim()) {
      return this.oauthError('invalid_request', 'install_nonce is required');
    }

    const subject = this.makeDefaultSubject({
      name: 'Install User',
      email: `install+${installNonce.slice(0, 8)}@git-ai.local`,
    });

    return this.issueTokenResponse(subject);
  }

  decodeAccessToken(accessToken: string): Record<string, unknown> | null {
    try {
      const payload = this.jwtService.verify(accessToken) as Record<string, unknown>;
      if (payload.type && payload.type !== 'access') {
        throw new UnauthorizedException('Unexpected token type');
      }
      return payload;
    } catch (_error) {
      return null;
    }
  }

  async getDeviceCodeByUserCode(userCode: string) {
    await this.pruneExpiredDeviceCodes();

    const entry = await this.findDeviceCodeByUserCode(userCode);
    if (!entry) {
      return null;
    }

    return {
      userCode: entry.userCode,
      expiresAt: entry.expiresAt,
      status: entry.status,
      subject: entry.subject,
    };
  }

  async approveDeviceCode(userCode: string) {
    await this.pruneExpiredDeviceCodes();

    const entry = await this.findDeviceCodeByUserCode(userCode);
    if (!entry) {
      return null;
    }

    if (entry.status === 'denied') {
      return {
        userCode: entry.userCode,
        expiresAt: entry.expiresAt,
        status: entry.status,
        subject: entry.subject,
      };
    }

    if (entry.status !== 'approved') {
      await this.prisma.$executeRaw`
        update public.oauth_device_codes
        set status = 'approved',
            approved_at = now()
        where user_code = ${userCode}
      `;
    }

    return {
      userCode: entry.userCode,
      expiresAt: entry.expiresAt,
      status: 'approved' as DeviceCodeStatus,
      subject: entry.subject,
    };
  }

  async denyDeviceCode(userCode: string) {
    await this.pruneExpiredDeviceCodes();

    const entry = await this.findDeviceCodeByUserCode(userCode);
    if (!entry) {
      return null;
    }

    if (entry.status !== 'approved') {
      await this.prisma.$executeRaw`
        update public.oauth_device_codes
        set status = 'denied',
            denied_at = now()
        where user_code = ${userCode}
      `;
    }

    return {
      userCode: entry.userCode,
      expiresAt: entry.expiresAt,
      status: entry.status === 'approved' ? entry.status : ('denied' as DeviceCodeStatus),
      subject: entry.subject,
    };
  }

  issueBrowserSessionToken(subject: Record<string, unknown> | TokenSubject) {
    return this.issueAccessToken(
      this.isTokenSubject(subject) ? subject : this.subjectFromPayload(subject),
    );
  }

  getAccessTokenTtlSeconds() {
    return this.accessTokenTtlSeconds;
  }

  private issueTokenResponse(subject: TokenSubject) {
    const refreshPayload = {
      sub: subject.sub,
      email: subject.email,
      name: subject.name,
      role: subject.role,
      type: 'refresh',
    };

    return {
      access_token: this.issueAccessToken(subject),
      token_type: 'Bearer',
      expires_in: this.accessTokenTtlSeconds,
      refresh_token: this.jwtService.sign(refreshPayload, {
        expiresIn: this.refreshTokenTtlSeconds,
      }),
      refresh_expires_in: this.refreshTokenTtlSeconds,
    };
  }

  private issueAccessToken(subject: TokenSubject) {
    const accessPayload = {
      ...subject,
      type: 'access',
    };

    return this.jwtService.sign(accessPayload, {
      expiresIn: this.accessTokenTtlSeconds,
    });
  }

  private isTokenSubject(subject: Record<string, unknown> | TokenSubject): subject is TokenSubject {
    return (
      typeof subject.sub === 'string' &&
      typeof subject.email === 'string' &&
      typeof subject.name === 'string' &&
      typeof subject.personal_org_id === 'string' &&
      Array.isArray(subject.orgs) &&
      (subject.role === 'admin' ||
        subject.role === 'user' ||
        subject.role === 'moderator' ||
        subject.role === 'guest')
    );
  }

  private oauthError(error: string, errorDescription?: string) {
    return {
      error,
      error_description: errorDescription,
    };
  }

  private subjectFromPayload(payload: Record<string, unknown>): TokenSubject {
    return {
      sub: typeof payload.sub === 'string' ? payload.sub : this.makeDefaultSubject().sub,
      email:
        typeof payload.email === 'string'
          ? payload.email
          : this.makeDefaultSubject().email,
      name:
        typeof payload.name === 'string'
          ? payload.name
          : this.makeDefaultSubject().name,
      personal_org_id:
        typeof payload.personal_org_id === 'string'
          ? payload.personal_org_id
          : this.makeDefaultSubject().personal_org_id,
      orgs: Array.isArray(payload.orgs)
        ? (payload.orgs as TokenSubject['orgs'])
        : this.makeDefaultSubject().orgs,
      role:
        payload.role === 'admin' ||
        payload.role === 'user' ||
        payload.role === 'moderator' ||
        payload.role === 'guest'
          ? (payload.role as CompatRole)
          : this.makeDefaultSubject().role,
    };
  }

  private async pruneExpiredDeviceCodes() {
    await this.prisma.$executeRaw`
      delete from public.oauth_device_codes
      where expires_at <= now()
    `;
  }

  private async findDeviceCodeByDeviceCode(deviceCode: string) {
    const rows = await this.prisma.$queryRaw<DeviceCodeRecord[]>`
      select
        device_code,
        user_code,
        status,
        expires_at,
        approved_at,
        denied_at,
        last_polled_at,
        subject_json::text as subject_json
      from public.oauth_device_codes
      where device_code = ${deviceCode}
      limit 1
    `;

    return this.mapDeviceCodeRecord(rows[0]);
  }

  private async findDeviceCodeByUserCode(userCode: string) {
    const rows = await this.prisma.$queryRaw<DeviceCodeRecord[]>`
      select
        device_code,
        user_code,
        status,
        expires_at,
        approved_at,
        denied_at,
        last_polled_at,
        subject_json::text as subject_json
      from public.oauth_device_codes
      where user_code = ${userCode}
      limit 1
    `;

    return this.mapDeviceCodeRecord(rows[0]);
  }

  private async touchDeviceCodePoll(deviceCode: string, timestamp: Date) {
    await this.prisma.$executeRaw`
      update public.oauth_device_codes
      set last_polled_at = ${timestamp}
      where device_code = ${deviceCode}
    `;
  }

  private async deleteDeviceCode(deviceCode: string) {
    await this.prisma.$executeRaw`
      delete from public.oauth_device_codes
      where device_code = ${deviceCode}
    `;
  }

  private mapDeviceCodeRecord(record?: DeviceCodeRecord) {
    if (!record) {
      return null;
    }

    return {
      deviceCode: record.device_code,
      userCode: record.user_code,
      expiresAt: record.expires_at.getTime(),
      status: record.status,
      approvedAt: record.approved_at,
      deniedAt: record.denied_at,
      lastPolledAt: record.last_polled_at,
      subject: this.parseStoredSubject(record.subject_json),
    };
  }

  private parseStoredSubject(serialized: string): TokenSubject {
    try {
      const parsed = JSON.parse(serialized) as Record<string, unknown>;
      return this.subjectFromPayload(parsed);
    } catch {
      return this.makeDefaultSubject();
    }
  }

  private makeUserCode() {
    const letters = crypto.randomBytes(4).toString('hex').slice(0, 4).toUpperCase();
    const digits = crypto.randomInt(1000, 9999).toString();
    return `${letters}-${digits}`;
  }

  private makeDefaultSubject(overrides?: Partial<TokenSubject>): TokenSubject {
    const base: TokenSubject = {
      sub: process.env.DEFAULT_USER_ID || '00000000-0000-0000-0000-000000000001',
      email: process.env.DEFAULT_USER_EMAIL || 'git-ai@example.local',
      name: process.env.DEFAULT_USER_NAME || 'Git AI User',
      personal_org_id: process.env.DEFAULT_PERSONAL_ORG_ID || 'git-ai-local-org',
      orgs: [
        {
          org_id: process.env.DEFAULT_PERSONAL_ORG_ID || 'git-ai-local-org',
          org_name: process.env.DEFAULT_ORG_NAME || 'Git AI Local',
          org_slug: process.env.DEFAULT_ORG_SLUG || 'git-ai-local',
          role: (process.env.DEFAULT_USER_ROLE as CompatRole) || 'user',
        },
      ],
      role: (process.env.DEFAULT_USER_ROLE as CompatRole) || 'user',
    };

    return {
      ...base,
      ...overrides,
      orgs: overrides?.orgs || base.orgs,
    };
  }
}
