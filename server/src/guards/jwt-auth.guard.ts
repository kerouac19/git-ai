import { Injectable, ExecutionContext } from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';
import { Reflector } from '@nestjs/core';
import { Logger } from '@nestjs/common';

@Injectable()
export class JwtAuthGuard extends AuthGuard('jwt') {
  private readonly logger = new Logger(JwtAuthGuard.name);
  constructor(private reflector: Reflector) {
    super();
  }

  canActivate(context: ExecutionContext) {
    // 获取是否跳过认证的装饰器
    const isPublic = this.reflector.getAllAndOverride<boolean>('public', [
      context.getHandler(),
      context.getClass(),
    ]);
    
    if (isPublic) {
      return true; // 如果标记为公共访问则跳过认证
    }

    // 否则执行JWT认证
    const canActivate = super.canActivate(context);
    if (typeof canActivate === 'boolean' && !canActivate) {
      this.logger.warn('Authentication failed for route');
    }
    
    return canActivate;
  }
}