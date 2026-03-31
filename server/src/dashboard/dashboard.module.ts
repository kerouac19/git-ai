import { Module } from '@nestjs/common';
import { DashboardService } from './dashboard.service';
import { DashboardController } from './dashboard.controller';
import { AggregatedMetricsService } from './aggregated-metrics.service';
import { PrismaModule } from '../prisma/prisma.module';
import { MetricsModule } from '../metrics/metrics.module';

@Module({
  imports: [PrismaModule, MetricsModule],
  controllers: [DashboardController],
  providers: [DashboardService, AggregatedMetricsService],
  exports: [DashboardService, AggregatedMetricsService],
})
export class DashboardModule {}
