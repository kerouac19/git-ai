import { Injectable, NestMiddleware, Logger } from '@nestjs/common';
import { Request, Response, NextFunction } from 'express';
import { PrismaService } from '../prisma/prisma.service';

export interface AuditLogEntry {
  id?: string;
  userId?: string;
  action: string;
  resource: string;
  params: any;
  ip: string;
  userAgent?: string;
  timestamp: Date;
  success: boolean;
  details?: string;
}

@Injectable()
export class AuditLogMiddleware implements NestMiddleware {
  private readonly logger = new Logger(AuditLogMiddleware.name);

  constructor(private readonly prisma: PrismaService) {}

  use(req: Request, res: Response, next: NextFunction) {
    const start = Date.now();
    
    // 记录审计日志的时间
    res.on('finish', async () => {
      const duration = Date.now() - start;
      
      // 配置是否记录审计日志
      const auditEnabled = await this.isAuditEnabled();
      
      if (auditEnabled) {
        this.logRequest(req, res, duration);
      }
    });

    next();
  }

  private async logRequest(req: Request, res: Response, duration: number) {
    // 获取用户ID (如果有登录验证)
    const userId = req['user']?.id || 'anonymous';

    // 创建审计日志条目
    const auditLog: AuditLogEntry = {
      userId,
      action: `${req.method} ${req.path}`,
      resource: req.path,
      params: {
        query: req.query,
        body: this.sanitizeBody(req.body),
        headers: this.getSafeHeaders(req.headers)
      },
      ip: this.getClientIp(req),
      userAgent: req.headers['user-agent'],
      timestamp: new Date(),
      success: res.statusCode < 400,
      details: `Duration: ${duration}ms, Status: ${res.statusCode}`
    };

    // 根据安全策略决定在哪里存储审计日志（数据库、文件系统、外部服务等）
    try {
      await this.storeAuditLog(auditLog);
      this.logger.log(`Audit event recorded: ${auditLog.action} by ${auditLog.userId}`);
    } catch (error) {
      // 记录审计日志失败，但不影响主要操作
      this.logger.error(`Failed to record audit log: ${error.message}`, error.stack);
    }
  }

  private sanitizeBody(body: any): any {
    if (!body || typeof body !== 'object') {
      return body;
    }

    const sanitized = { ...body };
    
    // 移除可能包含敏感信息的关键字
    const sensitiveKeys = ['password', 'token', 'secret', 'key', 'authorization', 'auth'];
    
    for (const key of sensitiveKeys) {
      if (sanitized.hasOwnProperty(key)) {
        sanitized[key] = '[REDACTED]';
      }
    }

    // 递归处理嵌套对象
    for (const [nestedKey, nestedValue] of Object.entries(sanitized)) {
      if (typeof nestedValue === 'object' && nestedValue !== null && !Array.isArray(nestedValue)) {
        sanitized[nestedKey] = this.sanitizeBody(nestedValue);
      }
    }

    return sanitized;
  }

  private getSafeHeaders(headers: any): any {
    const safeHeaders = {};
    const sensitiveHeaders = ['authorization', 'cookie', 'x-api-key', 'x-auth-token'];

    for (const [header, value] of Object.entries(headers)) {
      if (sensitiveHeaders.includes(header.toLowerCase())) {
        safeHeaders[header] = '[REDACTED]';
      } else {
        safeHeaders[header] = value;
      }
    }

    return safeHeaders;
  }

  private getClientIp(req: Request): string {
    return req.ip || 
           req.headers['x-forwarded-for']?.toString()?.split(',')[0] || 
           req.headers['x-real-ip']?.toString() || 
           req.connection.remoteAddress || 
           req.socket.remoteAddress ||
           'unknown';
  }

  private async storeAuditLog(log: AuditLogEntry) {
    await this.prisma.$executeRaw`
      insert into public.audit_logs (
        user_id,
        action,
        resource,
        params_json,
        ip,
        user_agent,
        occurred_at,
        success,
        details
      )
      values (
        ${log.userId || 'anonymous'},
        ${log.action},
        ${log.resource},
        ${JSON.stringify(log.params)}::jsonb,
        ${log.ip},
        ${log.userAgent || null},
        ${log.timestamp},
        ${log.success},
        ${log.details || null}
      )
    `;
  }

  private async isAuditEnabled(): Promise<boolean> {
    try {
      const config = await this.prisma.config.findUnique({
        where: { key: 'enable_audit_logs' },
      });
      
      return config ? config.value === 'true' || config.value === '1' : true;
    } catch (error) {
      this.logger.warn(`Could not determine if audit log is enabled, defaulting to true: ${error.message}`);
      return true; // 默认开启审计
    }
  }
}
