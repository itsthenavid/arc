import 'package:arc/core/config/app_config.dart';
import 'package:arc/core/platform/platform_info.dart';
import 'package:arc/features/auth/data/api_exception.dart';
import 'package:arc/features/auth/data/csrf_cookie_reader.dart';
import 'package:arc/features/auth/domain/auth_models.dart';
import 'package:dio/dio.dart';

abstract class AuthApiClient {
  Future<ArcAuthBundle> login({
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  });

  Future<ArcAuthBundle> consumeInvite({
    required String inviteToken,
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  });

  Future<ArcSession> refresh({String? refreshToken, required bool rememberMe});

  Future<ArcUser> me({required String accessToken});

  Future<void> logout({required String accessToken});

  Future<void> logoutAll({required String accessToken});

  Future<ArcInvite> createInvite({
    required String accessToken,
    int maxUses = 1,
    Duration? ttl,
  });
}

class DioAuthApiClient implements AuthApiClient {
  DioAuthApiClient({
    required Dio dio,
    required AppConfig config,
    required PlatformInfo platform,
    required CsrfCookieReader csrfCookieReader,
  }) : _dio = dio,
       _config = config,
       _platform = platform,
       _csrfCookieReader = csrfCookieReader;

  final Dio _dio;
  final AppConfig _config;
  final PlatformInfo _platform;
  final CsrfCookieReader _csrfCookieReader;

  @override
  Future<ArcAuthBundle> login({
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) async {
    final payload = <String, dynamic>{
      'password': password,
      'remember_me': rememberMe,
      'platform': _platform.apiPlatform,
    };

    if (username != null && username.trim().isNotEmpty) {
      payload['username'] = username.trim();
    }
    if (email != null && email.trim().isNotEmpty) {
      payload['email'] = email.trim();
    }

    try {
      final res = await _dio.post<dynamic>('/auth/login', data: payload);
      final json = _toJsonMap(res.data);
      final user = ArcUser.fromJson(_toJsonMap(json['user']));
      final session = ArcSession.fromJson(_toJsonMap(json['session']));
      return ArcAuthBundle(user: user, session: session);
    } on DioException catch (e) {
      throw _mapDioError(e);
    }
  }

  @override
  Future<ArcAuthBundle> consumeInvite({
    required String inviteToken,
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) async {
    final payload = <String, dynamic>{
      'invite_token': inviteToken.trim(),
      'password': password,
      'remember_me': rememberMe,
      'platform': _platform.apiPlatform,
    };

    if (username != null && username.trim().isNotEmpty) {
      payload['username'] = username.trim();
    }
    if (email != null && email.trim().isNotEmpty) {
      payload['email'] = email.trim();
    }

    try {
      final res = await _dio.post<dynamic>(
        '/auth/invites/consume',
        data: payload,
      );
      final json = _toJsonMap(res.data);
      final user = ArcUser.fromJson(_toJsonMap(json['user']));
      final session = ArcSession.fromJson(_toJsonMap(json['session']));
      return ArcAuthBundle(user: user, session: session);
    } on DioException catch (e) {
      throw _mapDioError(e);
    }
  }

  @override
  Future<ArcSession> refresh({
    String? refreshToken,
    required bool rememberMe,
  }) async {
    final payload = <String, dynamic>{
      'remember_me': rememberMe,
      'platform': _platform.apiPlatform,
    };

    if (refreshToken != null && refreshToken.trim().isNotEmpty) {
      payload['refresh_token'] = refreshToken.trim();
    }

    final headers = <String, dynamic>{};
    if (_config.webCookieMode) {
      final csrf = _csrfCookieReader.readCookie(_config.csrfCookieName);
      if (csrf != null && csrf.isNotEmpty) {
        headers[_config.csrfHeaderName] = csrf;
      }
    }

    try {
      final res = await _dio.post<dynamic>(
        '/auth/refresh',
        data: payload,
        options: Options(headers: headers.isEmpty ? null : headers),
      );
      final json = _toJsonMap(res.data);
      return ArcSession.fromJson(_toJsonMap(json['session']));
    } on DioException catch (e) {
      throw _mapDioError(e);
    }
  }

  @override
  Future<ArcUser> me({required String accessToken}) async {
    try {
      final res = await _dio.get<dynamic>(
        '/me',
        options: _bearerOptions(accessToken),
      );
      final json = _toJsonMap(res.data);
      return ArcUser.fromJson(_toJsonMap(json['user']));
    } on DioException catch (e) {
      throw _mapDioError(e);
    }
  }

  @override
  Future<void> logout({required String accessToken}) async {
    try {
      await _dio.post<dynamic>(
        '/auth/logout',
        options: _bearerOptions(accessToken),
      );
    } on DioException catch (e) {
      throw _mapDioError(e);
    }
  }

  @override
  Future<void> logoutAll({required String accessToken}) async {
    try {
      await _dio.post<dynamic>(
        '/auth/logout_all',
        options: _bearerOptions(accessToken),
      );
    } on DioException catch (e) {
      throw _mapDioError(e);
    }
  }

  @override
  Future<ArcInvite> createInvite({
    required String accessToken,
    int maxUses = 1,
    Duration? ttl,
  }) async {
    final payload = <String, dynamic>{'max_uses': maxUses};
    if (ttl != null && ttl > Duration.zero) {
      payload['expires_in_seconds'] = ttl.inSeconds;
    }

    try {
      final res = await _dio.post<dynamic>(
        '/auth/invites/create',
        data: payload,
        options: _bearerOptions(accessToken),
      );
      final json = _toJsonMap(res.data);
      return ArcInvite.fromJson(json);
    } on DioException catch (e) {
      throw _mapDioError(e);
    }
  }

  Options _bearerOptions(String accessToken) {
    return Options(headers: {'Authorization': 'Bearer $accessToken'});
  }

  ApiException _mapDioError(DioException error) {
    final statusCode = error.response?.statusCode;
    final data = error.response?.data;
    final path = error.requestOptions.path;
    final method = error.requestOptions.method.toUpperCase();

    if (data is Map<String, dynamic>) {
      final err = data['error'];
      if (err is Map<String, dynamic>) {
        final code = err['code'] as String?;
        final message = err['message'] as String?;
        final kind = _kindFromStatusOrCode(statusCode, code);
        return ApiException(
          message: (message == null || message.trim().isEmpty)
              ? 'Request failed'
              : message,
          kind: kind,
          code: code,
          statusCode: statusCode,
          requestPath: path,
          method: method,
          cause: error.error,
        );
      }
    }

    if (data is String && data.trim().isNotEmpty && statusCode != null) {
      return ApiException(
        message: _friendlyMessage(statusCode, path, data.trim()),
        kind: _kindFromStatusOrCode(statusCode, null),
        statusCode: statusCode,
        requestPath: path,
        method: method,
        cause: error.error,
      );
    }

    final kind = _kindFromDioType(error.type);
    final message = _messageFromDio(
      kind: kind,
      statusCode: statusCode,
      path: path,
      fallback: error.message,
    );

    return ApiException(
      message: message,
      kind: kind,
      statusCode: statusCode,
      requestPath: path,
      method: method,
      cause: error.error,
    );
  }

  Map<String, dynamic> _toJsonMap(Object? raw) {
    if (raw is Map<String, dynamic>) {
      return raw;
    }
    if (raw is Map) {
      return raw.map((key, value) => MapEntry(key.toString(), value));
    }
    throw const ApiException(
      message: 'Invalid server response format.',
      kind: ApiErrorKind.invalidResponse,
    );
  }

  ApiErrorKind _kindFromStatusOrCode(int? statusCode, String? code) {
    switch (statusCode) {
      case 400:
        return ApiErrorKind.invalidRequest;
      case 401:
        return ApiErrorKind.unauthorized;
      case 403:
        return ApiErrorKind.forbidden;
      case 409:
        return ApiErrorKind.conflict;
      case 429:
      case 503:
        return ApiErrorKind.unavailable;
      case 500:
      case 502:
      case 504:
        return ApiErrorKind.server;
      default:
        break;
    }

    switch (code) {
      case 'invalid_request':
      case 'invalid_json':
      case 'invalid_invite':
      case 'csrf_invalid':
        return ApiErrorKind.invalidRequest;
      case 'unauthorized':
      case 'invalid_credentials':
      case 'session_not_active':
      case 'refresh_reuse_detected':
        return ApiErrorKind.unauthorized;
      case 'captcha_invalid':
      case 'email_not_verified':
        return ApiErrorKind.forbidden;
      case 'conflict':
        return ApiErrorKind.conflict;
      case 'server_busy':
      case 'db_unavailable':
      case 'refresh_rate_limited':
        return ApiErrorKind.unavailable;
      case 'server_error':
        return ApiErrorKind.server;
      default:
        return ApiErrorKind.unknown;
    }
  }

  ApiErrorKind _kindFromDioType(DioExceptionType type) {
    switch (type) {
      case DioExceptionType.connectionTimeout:
      case DioExceptionType.sendTimeout:
      case DioExceptionType.receiveTimeout:
        return ApiErrorKind.timeout;
      case DioExceptionType.connectionError:
        return ApiErrorKind.network;
      case DioExceptionType.cancel:
        return ApiErrorKind.cancelled;
      case DioExceptionType.badResponse:
        return ApiErrorKind.unknown;
      case DioExceptionType.badCertificate:
      case DioExceptionType.unknown:
        return ApiErrorKind.unknown;
    }
  }

  String _messageFromDio({
    required ApiErrorKind kind,
    required int? statusCode,
    required String path,
    String? fallback,
  }) {
    if (statusCode != null) {
      return _friendlyMessage(statusCode, path, fallback);
    }

    switch (kind) {
      case ApiErrorKind.network:
        return 'Unable to connect to Arc server. Check if the server is running and ARC_API_BASE_URL is correct.';
      case ApiErrorKind.timeout:
        return 'Request timed out. The server may be overloaded or unreachable.';
      case ApiErrorKind.cancelled:
        return 'Request was cancelled.';
      default:
        return fallback?.trim().isNotEmpty == true
            ? fallback!.trim()
            : 'Request failed.';
    }
  }

  String _friendlyMessage(int statusCode, String path, String? fallback) {
    if (statusCode == 404 && path.startsWith('/auth/')) {
      return 'Auth API endpoint not found. Start the Go server with database and auth config enabled.';
    }
    if (statusCode == 503) {
      return 'Server is currently unavailable. Ensure database and auth services are configured.';
    }
    if (statusCode == 429) {
      return 'Too many requests. Please retry after a short delay.';
    }

    final cleanFallback = fallback?.trim();
    if (cleanFallback != null && cleanFallback.isNotEmpty) {
      return cleanFallback;
    }
    return 'Request failed (HTTP $statusCode).';
  }
}
