import { Controller, Get, Post, Body, Query, Param, UseGuards, Request, HttpException, HttpStatus } from '@nestjs/common';
import { DashboardService } from './dashboard.service';
import { DashboardDto } from './dashboard.dto';

@Controller('dashboard')
export class DashboardController {
  constructor(private readonly dashboardService: DashboardService) {}

  @Get('stats')
  async getUserDashboardStats(@Request() req: any, @Query('userId') userId: string) {
    // 简单的用户ID校验（完整版需要JWT Auth Guard）
    if (!userId) {
      throw new HttpException('User ID is required', HttpStatus.BAD_REQUEST);
    }

    // 在真实实现中，我们会从JWT token中提取用户ID并验证
    // 这里是示例实现
    try {
      const stats = await this.dashboardService.getDashboardStats(userId);
      
      return {
        success: true,
        data: stats,
        timestamp: new Date().toISOString(),
      };
    } catch (error: any) {
      console.error('Error getting dashboard stats:', error);
      throw new HttpException(
        'Failed to get dashboard statistics: ' + (error?.message || 'Unknown error'),
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  @Get('public')
  async getPublicStats() {
    try {
      const stats = await this.dashboardService.getPublicStats();
      
      return {
        success: true,
        data: stats,
        timestamp: new Date().toISOString(),
      };
    } catch (error: any) {
      console.error('Error getting public stats:', error);
      throw new HttpException(
        'Failed to get public statistics: ' + (error?.message || 'Unknown error'),
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  @Post('generate-report')
  async generateReport(@Body() reportParams: any) {
    // 生成自定义报告的端点
    // 实际情况下可能需要更多的参数验证
    try {
      // 实际实现可能需要复杂的业务逻辑
      console.log('Generating report with params:', reportParams);
      
      return {
        success: true,
        message: 'Report generation initiated',
        reportId: 'sample-report-id', // 实际实现会生成唯一的报告ID
      };
    } catch (error: any) {
      console.error('Error generating report:', error);
      throw new HttpException(
        'Failed to generate report: ' + (error?.message || 'Unknown error'),
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }
}