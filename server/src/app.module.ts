import { Module } from '@nestjs/common';
import { CasModule } from './cas/cas.module';
import { AuthorshipModule } from './authorship/authorship.module';
import { DashboardModule } from './dashboard/dashboard.module';
import { ConfigModule } from './config/config.module';
import { SecurityModule } from './security/security.module';
import { CompatibilityModule } from './compatibility/compatibility.module';
import { AuthModule } from './auth/auth.module';
import { PrismaModule } from './prisma/prisma.module';
import { MetricsModule } from './metrics/metrics.module';

@Module({
  imports: [
    PrismaModule,
    MetricsModule,
    CasModule,
    AuthorshipModule,
    DashboardModule,
    ConfigModule,
    SecurityModule,
    AuthModule,
    CompatibilityModule,
  ],
  controllers: [],
  providers: [],
})
export class AppModule {}
