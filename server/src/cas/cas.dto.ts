// src/server/cas/cas.dto.ts
import { IsString, IsOptional, MaxLength } from 'class-validator';

export class UploadDto {
  @IsString()
  content: string;

  @IsOptional()
  @IsString()
  contentType?: string;
}

export class ReadDto {
  @IsString()
  hash: string;
}