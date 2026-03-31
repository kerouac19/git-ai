import { Injectable } from '@nestjs/common';
import { AggregatedMetricsService } from './aggregated-metrics.service';
import { MetricsService } from '../metrics/metrics.service';

@Injectable()
export class DashboardService {
  constructor(
    private readonly aggregatedMetricsService: AggregatedMetricsService,
    private readonly metricsService: MetricsService,
  ) {}

  async getDashboardStats(userId: string) {
    // 获取聚合的仪表板统计信息
    const [stats, metricsSummary, casSummary] = await Promise.all([
      this.aggregatedMetricsService.getUserDashboardStats(userId),
      this.metricsService.getUserMetricsSummary(userId),
      this.aggregatedMetricsService.getCasRelatedMetrics(),
    ]);
    
    return {
      ...stats,
      metricsSummary,
      casSummary,
    };
  }

  async getDetailedStats(userId: string, options?: any) {
    // 获取详细的用户统计信息（带分页、过滤等选项）
    // 在实际实现中会有更多的业务逻辑
    const stats = await this.getDashboardStats(userId);
    
    // 可能添加额外的处理逻辑
    return stats;
  }

  async getPublicStats() {
    // 获取公共可见的统计数据
    const casMetrics = await this.aggregatedMetricsService.getCasRelatedMetrics();
    
    return {
      ...casMetrics,
      totalUsers: 0, // 在实际实施中需要从用户表读取
      systemHealth: 'operational',
    };
  }
}
