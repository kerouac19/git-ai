// src/server/dashboard/dashboard.dto.ts
import { IsString, IsNumber, IsObject, IsArray, ValidateNested, IsOptional } from 'class-validator';
import { Type } from 'class-transformer';

class UserInfoDto {
  @IsString()
  id: string;

  @IsString()
  username: string;

  @IsString()
  email: string;
}

class AiStatsDto {
  @IsNumber()
  totalTokens: number;

  @IsNumber()
  aiContributionPercentage: number;

  @IsNumber()
  totalAiLines: number;

  @IsNumber()
  aiSessionCount: number;
}

class GitActivityDto {
  @IsNumber()
  commitCount: number;

  @IsNumber()
  linesAdded: number;

  @IsNumber()
  linesDeleted: number;

  @IsNumber()
  fileChangeCount: number;
}

class TrendDataDto {
  @IsString()
  period: string; // week, month, quarter

  @IsArray()
  @IsNumber({}, { each: true })
  values: number[];
}

export class DashboardDto {
  @ValidateNested()
  @Type(() => UserInfoDto)
  userInfo: UserInfoDto;

  @ValidateNested()
  @Type(() => AiStatsDto)
  aiStats: AiStatsDto;

  @ValidateNested()
  @Type(() => GitActivityDto)
  gitActivity: GitActivityDto;

  @ValidateNested({ each: true })
  @Type(() => TrendDataDto)
  @IsArray()
  trends: TrendDataDto[];
}