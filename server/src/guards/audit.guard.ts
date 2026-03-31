import { Injectable, CanActivate, ExecutionContext, Logger, UnauthorizedException } from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { Observable } from 'rxjs';
import { AuditLogEntry } from '../security/audit-log.middleware';
import { PrismaService } from '../prisma/prisma.service';

@Injectable()
export class AuditGuard implements CanActivate {
  private readonly logger = new Logger(AuditGuard.name);

  constructor(
    private reflector: Reflector,
    private readonly prisma: PrismaService,
  ) {}

  canActivate(context: ExecutionContext): boolean | Promise<boolean> | Observable<boolean> {
    return this.executeAudit(context);
  }

  private async executeAudit(context: ExecutionContext): Promise<boolean> {
    const request = context.switchToHttp().getRequest();
    const response = context.switchToHttp().getResponse();
    
    // 获取需要记录审计的操作标识
    const auditRequired = this.reflector.getAllAndOverride<boolean>('audit', [
      context.getHandler(),
      context.getClass(),
    ]);

    if (!auditRequired) {
      return true; // 如果不需要审计，则总是允许通过
    }

    const user = request.user;

    // 对敏感操作进行详细记录
    const action = `${request.method} ${request.path}`;
    const resource = request.path;
    const userId = user?.id || user?.username || 'anonymous';
    
    // 记录审计信息
    const auditLog: AuditLogEntry = {
      userId,
      action,
      resource,
      params: {
        query: request.query,
        body: this.sanitizeParams(request.body),
      },
      ip: this.getClientIp(request),
      userAgent: request.headers['user-agent'],
      timestamp: new Date(),
      success: true,
      details: 'Security-sensitive operation audited'
    };

    try {
      // 存储审计日志
      await this.storeDetailedAuditLog(auditLog);
      
      this.logger.log(`Audit guard: Operation executed by ${userId} - ${action}`);
      
      // 检查是否存在可疑行为模式
      const isSuspicious = await this.detectAnomalousActivity(userId, action, auditLog.timestamp);
      
      if (isSuspicious) {
        this.logger.warn(`Suspicious activity detected for user ${userId}: ${action}`);
        // 这里可以触发告警或启用额外验证
        // 为演示目的，我们只是记录它
      }
      
      return true;
    } catch (error) {
      this.logger.error(`Audit guard failed: ${error.message}`);
      // 确保审计失败不会阻止操作继续，但会记录失败
      return true;
    }
  }

  private sanitizeParams(params: any): any {
    if (!params || typeof params !== 'object') {
      return params;
    }

    const sanitized = { ...params };
    
    // 移除可能包含敏感信息的关键字
    const sensitiveKeys = ['password', 'token', 'secret', 'key', 'authorization', 'auth'];
    
    for (const key of sensitiveKeys) {
      if (sanitized.hasOwnProperty(key)) {
        sanitized[key] = '[SANITIZED]';
      }
      // 检查小写变体
      if (sanitized.hasOwnProperty(key.toLowerCase())) {
        sanitized[key.toLowerCase()] = '[SANITIZED]';
      }
    }

    return sanitized;
  }

  private getClientIp(request: any): string {
    return request.ip || 
           request.headers['x-forwarded-for']?.toString()?.split(',')[0] || 
           request.headers['x-real-ip']?.toString() || 
           request.connection.remoteAddress || 
           request.socket.remoteAddress ||
           'unknown';
  }

  private async storeDetailedAuditLog(log: AuditLogEntry) {
    // 实际实现会将详细的审计日志存储在专门的审计数据库中
    console.log('SECURITY AUDIT:', JSON.stringify(log, null, 2));
    
    // 在生产环境中，这里可能调用专门的审计服务或写入安全的审计储存
  }

  /**
   * 检测异常活动模式
   * @param userId 用户ID
   * @param action 当前操作
   * @param timestamp 时间戳
   * @returns 如果检测到可疑活动返回true
   */
  private async detectAnomalousActivity(userId: string, action: string, timestamp: Date): Promise<boolean> {
    try {
      // 检查最近时间段内的登录失败频率
      // 当前兼容实现没有持久化 audit_log_table；这里只保留危险行为检测，
      // 并尝试从配置开关中读取是否启用更严格的异常检测。
      const config = await this.prisma.config.findUnique({
        where: { key: 'enable_anomaly_detection' },
      });
      const anomalyDetectionEnabled =
        !config || config.value === 'true' || config.value === '1';

      // 检查是否存在危险API调用
      const dangerousActions = ['/config/delete', '/users/change-permissions', '/db/raw-query'];
      if (anomalyDetectionEnabled && dangerousActions.some(dangerous => action.includes(dangerous))) {
        this.logger.warn(`Dangerous action executed: ${action} by user: ${userId}`);
        return true;
      }

      return false;

    } catch (error) {
      // 发生错误时，不阻塞操作，但记录错误
      this.logger.error(`Could not detect anomalous activity: ${error.message}`);
      return false;
    }
  }
}
