import 'dart:developer' as developer;

import 'package:flutter/foundation.dart';

class AppLog {
  AppLog._();

  static void debug(String message, {Map<String, Object?> fields = const {}}) {
    _write('DEBUG', message, fields: fields);
  }

  static void info(String message, {Map<String, Object?> fields = const {}}) {
    _write('INFO', message, fields: fields);
  }

  static void warn(String message, {Map<String, Object?> fields = const {}}) {
    _write('WARN', message, fields: fields);
  }

  static void error(
    String message, {
    Object? error,
    StackTrace? stackTrace,
    Map<String, Object?> fields = const {},
  }) {
    _write(
      'ERROR',
      message,
      fields: fields,
      error: error,
      stackTrace: stackTrace,
    );
  }

  static void _write(
    String level,
    String message, {
    required Map<String, Object?> fields,
    Object? error,
    StackTrace? stackTrace,
  }) {
    final payload = <String, Object?>{'level': level, ...fields};
    developer.log(
      message,
      name: 'arc.flutter',
      error: error,
      stackTrace: stackTrace,
      time: DateTime.now(),
      sequenceNumber: null,
      level: _numericLevel(level),
    );

    if (kDebugMode) {
      final text = payload.entries
          .map((entry) => '${entry.key}=${entry.value}')
          .join(' ');
      // ignore: avoid_print
      print('[arc][$level] $message${text.isEmpty ? '' : ' | $text'}');
      if (error != null) {
        // ignore: avoid_print
        print('[arc][$level] error=$error');
      }
      if (stackTrace != null) {
        // ignore: avoid_print
        print(stackTrace);
      }
    }
  }

  static int _numericLevel(String level) {
    switch (level) {
      case 'DEBUG':
        return 500;
      case 'INFO':
        return 800;
      case 'WARN':
        return 900;
      case 'ERROR':
        return 1000;
      default:
        return 0;
    }
  }
}
