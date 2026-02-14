import 'package:arc/app/app.dart';
import 'package:arc/core/runtime/app_logger.dart';
import 'package:arc/core/runtime/runtime_guard.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

Future<void> main() async {
  await runGuardedApp(() async {
    WidgetsFlutterBinding.ensureInitialized();
    AppLog.info('app.startup');

    runApp(const ProviderScope(child: ArcApp()));
  });
}
