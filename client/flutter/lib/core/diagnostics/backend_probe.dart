import 'dart:async';

import 'package:arc/core/diagnostics/backend_status.dart';
import 'package:arc/core/runtime/app_logger.dart';
import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

class BackendProbeController extends StateNotifier<BackendStatusState> {
  BackendProbeController({required Dio dio, Duration? interval})
    : _dio = dio,
      _interval = interval ?? const Duration(seconds: 12),
      super(BackendStatusState.initial()) {
    if (_interval > Duration.zero) {
      _timer = Timer.periodic(_interval, (_) {
        unawaited(checkNow());
      });
    }
    unawaited(checkNow());
  }

  BackendProbeController.disabled(super.initial)
    : _dio = null,
      _interval = Duration.zero;

  final Dio? _dio;
  final Duration _interval;
  Timer? _timer;
  BackendStatusLevel? _lastLoggedLevel;
  DateTime? _lastLoggedAt;

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  Future<void> checkNow() async {
    final dio = _dio;
    if (dio == null) {
      return;
    }

    final started = DateTime.now();
    try {
      final healthResponse = await dio.get<dynamic>(
        '/healthz',
        options: Options(
          sendTimeout: const Duration(seconds: 5),
          receiveTimeout: const Duration(seconds: 5),
          extra: const {'skip_auth': true, 'skip_http_log': true},
          responseType: ResponseType.plain,
        ),
      );
      final latency = DateTime.now().difference(started);

      if (healthResponse.statusCode == 200) {
        final next = BackendStatusState(
          level: BackendStatusLevel.online,
          message: 'Server reachable',
          latency: latency,
          checkedAt: DateTime.now(),
        );
        state = next;
        _logTransitionIfNeeded(next, note: 'healthz=200');
        return;
      }

      final next = BackendStatusState(
        level: BackendStatusLevel.degraded,
        message: 'Unexpected health status: ${healthResponse.statusCode}',
        latency: latency,
        checkedAt: DateTime.now(),
      );
      state = next;
      _logTransitionIfNeeded(
        next,
        note: 'healthz_status=${healthResponse.statusCode}',
      );
    } on DioException catch (e) {
      final next = BackendStatusState(
        level: BackendStatusLevel.offline,
        message: 'Server unreachable',
        checkedAt: DateTime.now(),
      );
      state = next;
      _logTransitionIfNeeded(
        next,
        note: 'type=${e.type.name} status=${e.response?.statusCode}',
      );
    } catch (e, st) {
      final next = BackendStatusState(
        level: BackendStatusLevel.offline,
        message: 'Server unreachable',
        checkedAt: DateTime.now(),
      );
      state = next;
      _logTransitionIfNeeded(next, note: 'unknown');
      AppLog.error('backend.health.check_unknown', error: e, stackTrace: st);
    }
  }

  void _logTransitionIfNeeded(BackendStatusState next, {required String note}) {
    final now = DateTime.now();
    final shouldLog =
        _lastLoggedLevel != next.level ||
        _lastLoggedAt == null ||
        now.difference(_lastLoggedAt!) >= const Duration(seconds: 60);
    if (!shouldLog) {
      return;
    }

    _lastLoggedLevel = next.level;
    _lastLoggedAt = now;

    if (next.level == BackendStatusLevel.online) {
      AppLog.info(
        'backend.health.online',
        fields: {'note': note, 'latency_ms': next.latency?.inMilliseconds},
      );
      return;
    }
    if (next.level == BackendStatusLevel.degraded) {
      AppLog.warn(
        'backend.health.degraded',
        fields: {'note': note, 'message': next.message},
      );
      return;
    }
    if (next.level == BackendStatusLevel.offline) {
      AppLog.warn('backend.health.offline', fields: {'note': note});
      return;
    }
    AppLog.info('backend.health.checking');
  }
}
