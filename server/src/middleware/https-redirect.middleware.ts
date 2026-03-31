import { Injectable, NestMiddleware, Logger } from '@nestjs/common';
import { Request, Response, NextFunction } from 'express';
import { SECURITY_CONFIG } from '../config/security.config';

function parseBooleanFlag(value?: string) {
  if (!value) {
    return false;
  }

  return ['1', 'true', 'yes', 'on'].includes(value.trim().toLowerCase());
}

@Injectable()
export class HttpsRedirectMiddleware implements NestMiddleware {
  private readonly logger = new Logger(HttpsRedirectMiddleware.name);

  use(req: Request, res: Response, next: NextFunction) {
    const httpsRedirectEnabled =
      typeof process.env.HTTPS_REDIRECT === 'string'
        ? parseBooleanFlag(process.env.HTTPS_REDIRECT)
        : SECURITY_CONFIG.HTTP_SECURITY.HTTPS_REDIRECT_ENABLED;
    const forwardedProto = req.headers['x-forwarded-proto'];
    const forwardedProtoValue = Array.isArray(forwardedProto)
      ? forwardedProto[0]
      : forwardedProto;
    const isForwardedHttps =
      typeof forwardedProtoValue === 'string' &&
      forwardedProtoValue
        .split(',')
        .some((value) => value.trim().toLowerCase() === 'https');
    const isSecure = req.secure || isForwardedHttps;

    if (httpsRedirectEnabled && !isSecure) {
      const host = req.get('Host');
      if (host) {
        const httpsUrl = `https://${host}${req.url}`;
        this.logger.log(`Redirecting to HTTPS: ${httpsUrl}`);
        return res.redirect(301, httpsUrl);
      }
    }

    // 设置安全头
    this.setSecurityHeaders(req, res, isSecure);
    
    next();
  }

  private setSecurityHeaders(_req: Request, res: Response, isSecure: boolean) {
    // HSTS (HTTP Strict Transport Security)
    if (SECURITY_CONFIG.HTTP_SECURITY.ENABLE_HSTS && isSecure) {
      res.setHeader('Strict-Transport-Security', SECURITY_CONFIG.HTTP_SECURITY.STRICT_TRANSPORT_SECURITY);
    }

    // XSS Protection
    if (SECURITY_CONFIG.HTTP_SECURITY.ENABLE_XSS_PROTECTION) {
      res.setHeader('X-XSS-Protection', '1; mode=block');
    }

    // Content Type Options
    if (SECURITY_CONFIG.HTTP_SECURITY.ENABLE_CONTENT_TYPE_OPTIONS) {
      res.setHeader('X-Content-Type-Options', 'nosniff');
    }

    // Frame Options
    if (SECURITY_CONFIG.HTTP_SECURITY.ENABLE_FRAME_OPTIONS) {
      res.setHeader('X-Frame-Options', 'DENY'); // 或者 SAMEORIGIN，根据需要
    }

    // Content Security Policy
    if (SECURITY_CONFIG.HTTP_SECURITY.ENABLE_CSP) {
      res.setHeader('Content-Security-Policy', this.getDefaultCSP());
    }

    // Referrer Policy
    res.setHeader('Referrer-Policy', 'strict-origin-when-cross-origin');

    // Permissions Policy
    res.setHeader('Permissions-Policy', 'geolocation=(), microphone=(), camera=()');
  }

  private getDefaultCSP(): string {
    return [
      "default-src 'self'",
      "script-src 'self' 'unsafe-inline'",
      "style-src 'self' 'unsafe-inline'",
      "img-src 'self' data:",
      "font-src 'self'",
      "connect-src 'self'",
      "frame-src 'self'",
      "object-src 'none'"
    ].join('; ');
  }
}
