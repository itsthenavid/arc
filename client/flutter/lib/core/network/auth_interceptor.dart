import 'package:dio/dio.dart';

typedef AccessTokenProvider = Future<String?> Function();
typedef RefreshAccessToken = Future<String?> Function();
typedef ShouldHandleRequest = bool Function(RequestOptions request);

class AuthTokenInterceptor extends QueuedInterceptor {
  AuthTokenInterceptor({
    required Dio dio,
    required AccessTokenProvider accessTokenProvider,
    required RefreshAccessToken refreshAccessToken,
    ShouldHandleRequest? shouldHandle,
  }) : _dio = dio,
       _accessTokenProvider = accessTokenProvider,
       _refreshAccessToken = refreshAccessToken,
       _shouldHandle = shouldHandle ?? _defaultShouldHandle;

  final Dio _dio;
  final AccessTokenProvider _accessTokenProvider;
  final RefreshAccessToken _refreshAccessToken;
  final ShouldHandleRequest _shouldHandle;

  static bool _defaultShouldHandle(RequestOptions request) {
    return !request.path.startsWith('/auth/');
  }

  @override
  Future<void> onRequest(
    RequestOptions options,
    RequestInterceptorHandler handler,
  ) async {
    if (!_shouldHandle(options) || options.extra['skip_auth'] == true) {
      handler.next(options);
      return;
    }

    try {
      final token = await _accessTokenProvider();
      if (token != null && token.isNotEmpty) {
        options.headers['Authorization'] = 'Bearer $token';
      }
    } catch (_) {
      // Request continues without an auth header when token resolution fails.
    }

    handler.next(options);
  }

  @override
  Future<void> onError(
    DioException err,
    ErrorInterceptorHandler handler,
  ) async {
    final request = err.requestOptions;
    final statusCode = err.response?.statusCode;
    final alreadyRetried = request.extra['retry_auth'] == true;

    if (statusCode != 401 ||
        alreadyRetried ||
        !_shouldHandle(request) ||
        request.extra['skip_auth'] == true) {
      handler.next(err);
      return;
    }

    try {
      final newAccessToken = await _refreshAccessToken();
      if (newAccessToken == null || newAccessToken.isEmpty) {
        handler.next(err);
        return;
      }

      final retriedOptions = _copyWithNewToken(request, newAccessToken);
      final response = await _dio.fetch<dynamic>(retriedOptions);
      handler.resolve(response);
      return;
    } catch (_) {
      handler.next(err);
      return;
    }
  }

  RequestOptions _copyWithNewToken(RequestOptions source, String accessToken) {
    final headers = Map<String, dynamic>.from(source.headers);
    headers['Authorization'] = 'Bearer $accessToken';

    return RequestOptions(
      path: source.path,
      method: source.method,
      baseUrl: source.baseUrl,
      data: source.data,
      queryParameters: Map<String, dynamic>.from(source.queryParameters),
      headers: headers,
      extra: {...source.extra, 'retry_auth': true},
      contentType: source.contentType,
      responseType: source.responseType,
      sendTimeout: source.sendTimeout,
      receiveTimeout: source.receiveTimeout,
      followRedirects: source.followRedirects,
      maxRedirects: source.maxRedirects,
      validateStatus: source.validateStatus,
      receiveDataWhenStatusError: source.receiveDataWhenStatusError,
      listFormat: source.listFormat,
    );
  }
}
