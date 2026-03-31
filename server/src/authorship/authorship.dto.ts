// src/server/authorship/authorship.dto.ts
import { IsString, IsArray, IsNumber, ValidateNested, IsObject, IsDateString, IsUUID } from 'class-validator';
import { Type } from 'class-transformer';

class FileAttributionDto {
  @IsString()
  filename: string;

  @IsArray()
  @IsNumber({}, { each: true })
  humanLines: number[];

  @IsArray()
  @IsNumber({}, { each: true })
  aiLines: number[];

  @IsNumber()
  aiPercentage: number;
}

class AiContributionMetricsDto {
  @IsNumber()
  aiLineCount: number;

  @IsNumber()
  totalLineCount: number;

  @IsNumber()
  aiPercentage: number;

  @IsNumber()
  tokensUsed: number;
}

export class AuthorshipDto {
  @IsUUID()
  userId: string;

  @IsString()
  gitCommitHash: string;

  @IsArray()
  @ValidateNested({ each: true })
  @Type(() => FileAttributionDto)
  fileAttributions: FileAttributionDto[];

  @IsNumber()
  aiAttributionPercentage: number;
  
  @IsDateString()
  timestamp?: string;
}

export class CommitAttributionDto {
  @IsString()
  commitHash: string;

  @IsString()
  author: string;

  @IsObject()
  fileChanges: Record<string, {
    additions: number;
    deletions: number;
    aiAffected: boolean;
  }>;

  @ValidateNested()
  @Type(() => AiContributionMetricsDto)
  aiContributionMetrics: AiContributionMetricsDto;
  
  @IsDateString()
  timestamp?: string;
}