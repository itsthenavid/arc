enum BackendStatusLevel { checking, online, degraded, offline }

class BackendStatusState {
  const BackendStatusState({
    required this.level,
    this.message,
    this.latency,
    this.checkedAt,
  });

  final BackendStatusLevel level;
  final String? message;
  final Duration? latency;
  final DateTime? checkedAt;

  bool get isOnline => level == BackendStatusLevel.online;

  factory BackendStatusState.initial() {
    return const BackendStatusState(level: BackendStatusLevel.checking);
  }

  BackendStatusState copyWith({
    BackendStatusLevel? level,
    String? message,
    Duration? latency,
    DateTime? checkedAt,
  }) {
    return BackendStatusState(
      level: level ?? this.level,
      message: message ?? this.message,
      latency: latency ?? this.latency,
      checkedAt: checkedAt ?? this.checkedAt,
    );
  }
}
