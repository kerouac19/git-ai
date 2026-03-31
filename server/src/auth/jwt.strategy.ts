import { Injectable } from '@nestjs/common';
import { PassportStrategy } from '@nestjs/passport';
import { ExtractJwt, Strategy } from 'passport-jwt';
import { extractAccessTokenFromCookieHeader, getJwtSecret } from './http-auth.util';

@Injectable()
export class JwtStrategy extends PassportStrategy(Strategy) {
  constructor() {
    super({
      jwtFromRequest: ExtractJwt.fromExtractors([
        ExtractJwt.fromAuthHeaderAsBearerToken(),
        (request: { headers?: { cookie?: string } }) =>
          extractAccessTokenFromCookieHeader(request?.headers?.cookie),
      ]),
      ignoreExpiration: false,
      secretOrKey: getJwtSecret(),
    });
  }

  validate(payload: Record<string, unknown>) {
    const role =
      payload.role === 'admin' ||
      payload.role === 'user' ||
      payload.role === 'moderator' ||
      payload.role === 'guest'
        ? payload.role
        : 'user';

    return {
      id: payload.sub,
      username: payload.name || payload.email,
      email: payload.email,
      role,
      personal_org_id: payload.personal_org_id,
      orgs: payload.orgs || [],
    };
  }
}
