import 'package:arc/core/config/app_config.dart';
import 'package:arc/core/network/configure_web_dio.dart';
import 'package:arc/core/runtime/app_logger.dart';
import 'package:dio/dio.dart';

Dio createBaseDio(AppConfig config) {
  final dio = Dio(
    BaseOptions(
      baseUrl: config.apiBaseUrl,
      connectTimeout: const Duration(seconds: 10),
      sendTimeout: const Duration(seconds: 15),
      receiveTimeout: const Duration(seconds: 20),
      responseType: ResponseType.json,
      headers: const {'Accept': 'application/json'},
    ),
  );

  configureWebDio(dio);
  dio.interceptors.add(
    InterceptorsWrapper(
      onRequest: (options, handler) {
        options.extra['arc_started_at'] = DateTime.now().microsecondsSinceEpoch;
        handler.next(options);
      },
      onResponse: (response, handler) {
        if (_skipHttpLog(response.requestOptions)) {
          handler.next(response);
          return;
        }
        final elapsedMs = _elapsedFromExtra(response.requestOptions.extra);
        AppLog.debug(
          'http.response',
          fields: {
            'method': response.requestOptions.method.toUpperCase(),
            'path': response.requestOptions.path,
            'status': response.statusCode,
            'duration_ms': elapsedMs,
          },
        );
        handler.next(response);
      },
      onError: (error, handler) {
        if (_skipHttpLog(error.requestOptions)) {
          handler.next(error);
          return;
        }
        final elapsedMs = _elapsedFromExtra(error.requestOptions.extra);
        AppLog.warn(
          'http.error',
          fields: {
            'method': error.requestOptions.method.toUpperCase(),
            'path': error.requestOptions.path,
            'status': error.response?.statusCode,
            'type': error.type.name,
            'duration_ms': elapsedMs,
          },
        );
        handler.next(error);
      },
    ),
  );
  return dio;
}

int _elapsedFromExtra(Map<String, dynamic> extra) {
  final raw = extra['arc_started_at'];
  if (raw is! int) {
    return -1;
  }
  final started = DateTime.fromMicrosecondsSinceEpoch(raw);
  return DateTime.now().difference(started).inMilliseconds;
}

bool _skipHttpLog(RequestOptions options) {
  return options.extra['skip_http_log'] == true;
}
