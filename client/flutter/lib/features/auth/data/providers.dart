import 'package:arc/core/config/app_config.dart';
import 'package:arc/core/diagnostics/backend_probe.dart';
import 'package:arc/core/diagnostics/backend_status.dart';
import 'package:arc/core/network/auth_interceptor.dart';
import 'package:arc/core/network/dio_factory.dart';
import 'package:arc/core/platform/platform_info.dart';
import 'package:arc/features/auth/data/auth_api.dart';
import 'package:arc/features/auth/data/auth_repository.dart';
import 'package:arc/features/auth/data/csrf_cookie_reader.dart';
import 'package:arc/features/auth/data/token_store.dart';
import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

final appConfigProvider = Provider<AppConfig>((ref) {
  return AppConfig.fromEnvironment();
});

final platformInfoProvider = Provider<PlatformInfo>((ref) {
  return PlatformInfo.current();
});

final authStoreProvider = Provider<AuthStore>((ref) {
  final platform = ref.watch(platformInfoProvider);
  return DeviceAuthStore(platform: platform);
});

final authApiProvider = Provider<AuthApiClient>((ref) {
  final config = ref.watch(appConfigProvider);
  final platform = ref.watch(platformInfoProvider);
  final effectiveWebCookieMode = config.webCookieMode && platform.isWeb;
  final dio = createBaseDio(config);

  return DioAuthApiClient(
    dio: dio,
    config: config.copyWith(webCookieMode: effectiveWebCookieMode),
    platform: platform,
    csrfCookieReader: createCsrfCookieReader(),
  );
});

final authRepositoryProvider = Provider<AuthRepository>((ref) {
  final api = ref.watch(authApiProvider);
  final store = ref.watch(authStoreProvider);
  final config = ref.watch(appConfigProvider);
  final platform = ref.watch(platformInfoProvider);

  return DefaultAuthRepository(
    api: api,
    store: store,
    webCookieMode: config.webCookieMode && platform.isWeb,
  );
});

final authorizedDioProvider = Provider<Dio>((ref) {
  final config = ref.watch(appConfigProvider);
  final repo = ref.watch(authRepositoryProvider);

  final dio = createBaseDio(config);
  dio.interceptors.add(
    AuthTokenInterceptor(
      dio: dio,
      accessTokenProvider: () async {
        try {
          final session = await repo.ensureActiveSession(
            skew: const Duration(seconds: 20),
          );
          return session.accessToken;
        } catch (_) {
          return null;
        }
      },
      refreshAccessToken: () async {
        try {
          final session = await repo.refreshSession();
          return session.accessToken;
        } catch (_) {
          return null;
        }
      },
      shouldHandle: (request) {
        if (request.extra['skip_auth'] == true) {
          return false;
        }
        return !request.path.startsWith('/auth/');
      },
    ),
  );

  return dio;
});

final backendProbeProvider =
    StateNotifierProvider<BackendProbeController, BackendStatusState>((ref) {
      final config = ref.watch(appConfigProvider);
      final dio = createBaseDio(config);
      return BackendProbeController(dio: dio);
    });
