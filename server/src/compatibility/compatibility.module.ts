import { Module } from '@nestjs/common';
import { CompatibilityController } from './compatibility.controller';
import { DashboardModule } from '../dashboard/dashboard.module';
import { AuthorshipModule } from '../authorship/authorship.module';
import { CasModule } from '../cas/cas.module';
import { AuthModule } from '../auth/auth.module';
import { MetricsModule } from '../metrics/metrics.module';

@Module({
  imports: [DashboardModule, AuthorshipModule, CasModule, AuthModule, MetricsModule],
  controllers: [CompatibilityController],
})
export class CompatibilityModule {}
