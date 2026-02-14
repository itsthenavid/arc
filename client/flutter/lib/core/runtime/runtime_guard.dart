import 'dart:async';
import 'dart:ui';

import 'package:arc/core/runtime/app_logger.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

Future<void> runGuardedApp(FutureOr<void> Function() start) async {
  final guarded = runZonedGuarded<Future<void>>(
    () async {
      FlutterError.onError = (details) {
        FlutterError.presentError(details);
        AppLog.error(
          'flutter.framework_error',
          error: details.exception,
          stackTrace: details.stack,
        );
      };

      PlatformDispatcher.instance.onError = (error, stackTrace) {
        AppLog.error(
          'flutter.platform_error',
          error: error,
          stackTrace: stackTrace,
        );
        return true;
      };

      ErrorWidget.builder = (details) {
        AppLog.error(
          'flutter.error_widget',
          error: details.exception,
          stackTrace: details.stack,
        );

        return Material(
          color: const Color(0xFF101720),
          child: Center(
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: Text(
                kReleaseMode
                    ? 'Something went wrong. Please restart the app.'
                    : details.exceptionAsString(),
                style: const TextStyle(color: Colors.white),
                textAlign: TextAlign.center,
              ),
            ),
          ),
        );
      };

      await start();
    },
    (error, stackTrace) {
      AppLog.error('flutter.zone_error', error: error, stackTrace: stackTrace);
    },
  );
  if (guarded != null) {
    await guarded;
  }
}
