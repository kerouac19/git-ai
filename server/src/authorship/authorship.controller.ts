import { Controller, Get, Post, Put, Body, Param, Query, HttpException, HttpStatus } from '@nestjs/common';
import { AuthorshipService } from './authorship.service';
import { AuthorshipDto, CommitAttributionDto } from './authorship.dto';

@Controller('authorship')
export class AuthorshipController {
  constructor(private readonly authorshipService: AuthorshipService) {}

  private parsePaginationValue(value: number | string | undefined, fallback: number) {
    const parsed = typeof value === 'number' ? value : Number(value);
    if (!Number.isFinite(parsed)) {
      return fallback;
    }

    return parsed < 0 ? 0 : Math.trunc(parsed);
  }

  @Post('record')
  async saveAuthorshipRecord(@Body() authorshipDto: AuthorshipDto) {
    try {
      const record = await this.authorshipService.saveAuthorshipRecord(authorshipDto);
      
      return {
        success: true,
        message: 'Authorship record saved',
        recordId: record.id
      };
    } catch (error: any) {
      console.error('Error saving authorship record:', error);
      throw new HttpException(
        'Failed to save authorship record: ' + (error?.message || 'Unknown error'),
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  @Post('commit')
  async saveCommitAttribution(@Body() commitAttributionDto: CommitAttributionDto) {
    try {
      const attribution = await this.authorshipService.saveCommitAttribution(commitAttributionDto);
      
      return {
        success: true,
        message: 'Commit attribution saved',
        attributionId: attribution.id
      };
    } catch (error: any) {
      console.error('Error saving commit attribution:', error);
      throw new HttpException(
        'Failed to save commit attribution: ' + (error?.message || 'Unknown error'),
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  @Get('commits/:userId')
  async getAuthorshipByUser(
    @Param('userId') userId: string,
    @Query('limit') limit: number = 50,
    @Query('offset') offset: number = 0
  ) {
    try {
      const safeLimit = this.parsePaginationValue(limit, 50);
      const safeOffset = this.parsePaginationValue(offset, 0);
      const result = await this.authorshipService.findAll(userId, safeLimit, safeOffset);
      
      return {
        success: true,
        data: result.records,
        total: result.total,
        limit: safeLimit,
        offset: safeOffset,
      };
    } catch (error: any) {
      console.error('Error retrieving authorship records:', error);
      throw new HttpException(
        'Failed to retrieve authorship records: ' + (error?.message || 'Unknown error'),
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  @Get('commits/:userId/:commitHash')
  async getAuthorshipByUserAndCommit(
    @Param('userId') userId: string,
    @Param('commitHash') commitHash: string
  ) {
    try {
      const records = await this.authorshipService.findByCommitHash(commitHash);
      const filteredRecords = records.filter(record => record.userId === userId);
      
      return {
        success: true,
        data: filteredRecords,
      };
    } catch (error: any) {
      console.error('Error retrieving authorship records:', error);
      throw new HttpException(
        'Failed to retrieve authorship records: ' + (error?.message || 'Unknown error'),
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  @Get('commit/:commitHash')
  async getCommitAttribution(@Param('commitHash') commitHash: string) {
    try {
      const attribution = await this.authorshipService.findCommitAttributionByHash(commitHash);
      
      if (!attribution) {
        throw new HttpException('Commit attribution not found', HttpStatus.NOT_FOUND);
      }

      return {
        success: true,
        data: attribution,
      };
    } catch (error: any) {
      if (error instanceof HttpException && error.getStatus() === HttpStatus.NOT_FOUND) {
        throw error;
      }
      console.error('Error retrieving commit attribution:', error);
      throw new HttpException(
        'Failed to retrieve commit attribution: ' + (error?.message || 'Unknown error'),
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  @Put('sync/:userId')
  async syncAuthorshipData(@Param('userId') userId: string, @Body() newAuthorshipData: AuthorshipDto) {
    try {
      const mergedRecord = await this.authorshipService.mergeAuthorshipData(userId, newAuthorshipData);
      
      return {
        success: true,
        message: 'Authorship data synced successfully',
        recordId: mergedRecord.id
      };
    } catch (error: any) {
      console.error('Error syncing authorship data:', error);
      throw new HttpException(
        'Failed to sync authorship data: ' + (error?.message || 'Unknown error'),
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }
}
