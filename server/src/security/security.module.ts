import { Module, MiddlewareConsumer, NestModule, RequestMethod } from '@nestjs/common';
import { EncryptionService } from './encryption.service';
import { AuditLogMiddleware } from './audit-log.middleware';
import { HttpsRedirectMiddleware } from '../middleware/https-redirect.middleware';
import { PermissionGuard } from '../guards/permission.guard';
import { AuditGuard } from '../guards/audit.guard';
import { EncryptionInterceptor } from '../interceptors/encryption.interceptor';
import { PrismaModule } from '../prisma/prisma.module';

@Module({
  imports: [PrismaModule],
  providers: [
    EncryptionService,
    AuditLogMiddleware,
    HttpsRedirectMiddleware,
    PermissionGuard,
    AuditGuard,
    EncryptionInterceptor,
  ],
  exports: [
    EncryptionService,
    PermissionGuard,
    AuditGuard,
    EncryptionInterceptor,
  ],
})
export class SecurityModule implements NestModule {
  configure(consumer: MiddlewareConsumer) {
    consumer
      .apply(HttpsRedirectMiddleware, AuditLogMiddleware)
      .forRoutes({ path: '{*splat}', method: RequestMethod.ALL });
  }
}
