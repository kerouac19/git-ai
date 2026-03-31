import { Module } from '@nestjs/common';
import { JwtModule } from '@nestjs/jwt';
import { PassportModule } from '@nestjs/passport';
import { CompatibilityAuthService } from './compatibility-auth.service';
import { JwtStrategy } from './jwt.strategy';
import { getJwtSecret } from './http-auth.util';

@Module({
  imports: [
    PassportModule.register({ defaultStrategy: 'jwt' }),
    JwtModule.register({
      secret: getJwtSecret(),
      signOptions: { algorithm: 'HS256' },
    }),
  ],
  providers: [CompatibilityAuthService, JwtStrategy],
  exports: [CompatibilityAuthService, JwtModule, PassportModule],
})
export class AuthModule {}
