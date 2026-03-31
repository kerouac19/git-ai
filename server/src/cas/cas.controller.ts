import { Controller, Get, Post, Body, Param, HttpException, HttpStatus } from '@nestjs/common';
import { CasService } from './cas.service';
import { UploadDto } from './cas.dto';

@Controller('cas')
export class CasController {
  constructor(private readonly casService: CasService) {}

  @Post('upload')
  async upload(@Body() uploadDto: UploadDto) {
    if (!uploadDto.content) {
      throw new HttpException('Content is required', HttpStatus.BAD_REQUEST);
    }

    try {
      // 上传内容并获取哈希
      const hash = await this.casService.uploadContent(uploadDto.content, uploadDto.contentType);
      
      return {
        hash,
        success: true,
        message: 'Content uploaded successfully',
        contentType: uploadDto.contentType || 'text/plain'
      };
    } catch (error: any) {
      console.error('Upload error:', error?.message || error);
      throw new HttpException(
        'Failed to upload content: ' + (error?.message || 'Unknown error'), 
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }

  @Get('read/:hash')
  async read(@Param('hash') hash: string) {
    if (!hash) {
      throw new HttpException('Hash is required', HttpStatus.BAD_REQUEST);
    }

    try {
      const result = await this.casService.readContent(hash);
      
      if (!result) {
        throw new HttpException('Content not found', HttpStatus.NOT_FOUND);
      }

      return {
        content: result.content,
        hash,
        success: true,
        contentType: result.contentType
      };
    } catch (error: any) {
      if (error instanceof HttpException && error.getStatus() === HttpStatus.NOT_FOUND) {
        throw error;
      }
      console.error('Read error:', error?.message || error);
      throw new HttpException(
        'Failed to read content: ' + (error?.message || 'Unknown error'), 
        HttpStatus.INTERNAL_SERVER_ERROR
      );
    }
  }
}