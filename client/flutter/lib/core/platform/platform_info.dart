import 'package:flutter/foundation.dart';

class PlatformInfo {
  const PlatformInfo({required this.isWeb, required this.targetPlatform});

  final bool isWeb;
  final TargetPlatform targetPlatform;

  factory PlatformInfo.current() {
    return PlatformInfo(isWeb: kIsWeb, targetPlatform: defaultTargetPlatform);
  }

  String get apiPlatform {
    if (isWeb) {
      return 'web';
    }

    return switch (targetPlatform) {
      TargetPlatform.iOS => 'ios',
      TargetPlatform.android => 'android',
      TargetPlatform.macOS ||
      TargetPlatform.windows ||
      TargetPlatform.linux => 'desktop',
      _ => 'unknown',
    };
  }

  bool get isDesktop {
    return !isWeb &&
        (targetPlatform == TargetPlatform.macOS ||
            targetPlatform == TargetPlatform.windows ||
            targetPlatform == TargetPlatform.linux);
  }
}
