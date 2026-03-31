import { Injectable, CanActivate, ExecutionContext } from '@nestjs/common';
import { Observable } from 'rxjs';
import { Reflector } from '@nestjs/core';
import { Logger } from '@nestjs/common';
import { ROLES_KEY } from './roles.decorator';

export enum Role {
  ADMIN = 'admin',
  USER = 'user',
  MODERATOR = 'moderator',
  GUEST = 'guest'
}

@Injectable()
export class PermissionGuard implements CanActivate {
  private readonly logger = new Logger(PermissionGuard.name);

  constructor(private reflector: Reflector) {}

  canActivate(context: ExecutionContext): boolean | Promise<boolean> | Observable<boolean> {
    // 未显式声明角色的端点默认拒绝，避免把权限判断建立在隐式默认值上。
    const requiredRoles = this.reflector.getAllAndOverride<Role[]>(ROLES_KEY, [
      context.getHandler(),
      context.getClass(),
    ]);
    
    // 获取请求对象
    const request = context.switchToHttp().getRequest();
    
    // 获取用户信息
    const user = request.user;

    // 如果没有用户信息，拒绝访问
    if (!user) {
      this.logger.warn('Access denied: No user information available in request');
      return false;
    }

    // 检查用户角色，如果用户是ADMIN，则授予访问权限
    if (user.role === Role.ADMIN) {
      this.logger.log(`Admin access granted to user ${user.id || user.username}`);
      return true;
    }

    if (!requiredRoles || requiredRoles.length === 0) {
      this.logger.warn(
        `Access denied to user ${user.id || user.username}: route is missing role metadata`,
      );
      return false;
    }

    // 检查用户是否具有所需的最低权限
    const hasPermission = this.checkPermission(user.role as Role, requiredRoles);
    
    if (hasPermission) {
      this.logger.log(`Access granted to user ${user.id || user.username} with role: ${user.role}`);
    } else {
      this.logger.warn(`Access denied to user ${user.id || user.username} with role: ${user.role}. Required: ${requiredRoles.join(', ')}`);
    }

    return hasPermission;
  }

  /**
   * 检查用户角色是否有权限访问资源
   * @param userRole 用户角色
   * @param requiredRoles 所需的最低角色集合
   * @returns 有权限返回true，否则返回false
   */
  private checkPermission(userRole: Role, requiredRoles: Role[]): boolean {
    // 如果没有定义所需角色，则默认拒绝
    if (!requiredRoles || requiredRoles.length === 0) {
      return false;
    }

    // 检查用户是否具有的角色在所需角色之中
    const rolesHierarchy = {
      [Role.GUEST]: 0,
      [Role.USER]: 1,
      [Role.MODERATOR]: 2,
      [Role.ADMIN]: 3
    };

    if (!(userRole in rolesHierarchy)) {
      return false;
    }

    return requiredRoles.some(requiredRole => {
      if (!(requiredRole in rolesHierarchy)) {
        return false;
      }
      return rolesHierarchy[userRole] >= rolesHierarchy[requiredRole];
    });
  }
}
