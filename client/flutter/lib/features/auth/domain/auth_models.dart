import 'dart:convert';

class ArcUser {
  const ArcUser({
    required this.id,
    required this.createdAt,
    this.username,
    this.email,
    this.emailVerifiedAt,
    this.displayName,
    this.bio,
  });

  final String id;
  final String? username;
  final String? email;
  final DateTime? emailVerifiedAt;
  final String? displayName;
  final String? bio;
  final DateTime createdAt;

  bool get needsProfileCompletion {
    return (displayName == null || displayName!.trim().isEmpty);
  }

  ArcUser copyWith({
    String? username,
    String? email,
    DateTime? emailVerifiedAt,
    String? displayName,
    String? bio,
  }) {
    return ArcUser(
      id: id,
      username: username ?? this.username,
      email: email ?? this.email,
      emailVerifiedAt: emailVerifiedAt ?? this.emailVerifiedAt,
      displayName: displayName ?? this.displayName,
      bio: bio ?? this.bio,
      createdAt: createdAt,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'username': username,
      'email': email,
      'email_verified_at': emailVerifiedAt?.toUtc().toIso8601String(),
      'display_name': displayName,
      'bio': bio,
      'created_at': createdAt.toUtc().toIso8601String(),
    };
  }

  String encode() => jsonEncode(toJson());

  static ArcUser decode(String raw) {
    final data = jsonDecode(raw) as Map<String, dynamic>;
    return ArcUser.fromJson(data);
  }

  factory ArcUser.fromJson(Map<String, dynamic> json) {
    return ArcUser(
      id: json['id'] as String,
      username: json['username'] as String?,
      email: json['email'] as String?,
      emailVerifiedAt: _parseOptionalTime(json['email_verified_at']),
      displayName: json['display_name'] as String?,
      bio: json['bio'] as String?,
      createdAt: DateTime.parse(json['created_at'] as String).toUtc(),
    );
  }
}

class ArcSession {
  const ArcSession({
    required this.sessionId,
    required this.accessToken,
    required this.accessExpiresAt,
    required this.refreshExpiresAt,
    required this.rememberMe,
    this.refreshToken,
  });

  final String sessionId;
  final String accessToken;
  final DateTime accessExpiresAt;
  final String? refreshToken;
  final DateTime refreshExpiresAt;
  final bool rememberMe;

  bool get hasRefreshToken => refreshToken != null && refreshToken!.isNotEmpty;

  bool isAccessExpired({Duration skew = Duration.zero}) {
    final threshold = DateTime.now().toUtc().add(skew);
    return !accessExpiresAt.isAfter(threshold);
  }

  ArcSession copyWith({
    String? sessionId,
    String? accessToken,
    DateTime? accessExpiresAt,
    String? refreshToken,
    DateTime? refreshExpiresAt,
    bool? rememberMe,
  }) {
    return ArcSession(
      sessionId: sessionId ?? this.sessionId,
      accessToken: accessToken ?? this.accessToken,
      accessExpiresAt: accessExpiresAt ?? this.accessExpiresAt,
      refreshToken: refreshToken ?? this.refreshToken,
      refreshExpiresAt: refreshExpiresAt ?? this.refreshExpiresAt,
      rememberMe: rememberMe ?? this.rememberMe,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'session_id': sessionId,
      'access_token': accessToken,
      'access_expires_at': accessExpiresAt.toUtc().toIso8601String(),
      'refresh_token': refreshToken,
      'refresh_expires_at': refreshExpiresAt.toUtc().toIso8601String(),
      'remember_me': rememberMe,
    };
  }

  String encode() => jsonEncode(toJson());

  static ArcSession decode(String raw) {
    final data = jsonDecode(raw) as Map<String, dynamic>;
    return ArcSession.fromJson(data);
  }

  factory ArcSession.fromJson(Map<String, dynamic> json) {
    final refreshToken = (json['refresh_token'] as String?)?.trim();
    return ArcSession(
      sessionId: (json['session_id'] as String?) ?? '',
      accessToken: (json['access_token'] as String?) ?? '',
      accessExpiresAt: DateTime.parse(
        json['access_expires_at'] as String,
      ).toUtc(),
      refreshToken: (refreshToken == null || refreshToken.isEmpty)
          ? null
          : refreshToken,
      refreshExpiresAt: DateTime.parse(
        json['refresh_expires_at'] as String,
      ).toUtc(),
      rememberMe: (json['remember_me'] as bool?) ?? false,
    );
  }
}

class ArcAuthBundle {
  const ArcAuthBundle({required this.user, required this.session});

  final ArcUser user;
  final ArcSession session;
}

class ArcInvite {
  const ArcInvite({
    required this.inviteId,
    required this.inviteToken,
    required this.expiresAt,
  });

  final String inviteId;
  final String inviteToken;
  final DateTime expiresAt;

  factory ArcInvite.fromJson(Map<String, dynamic> json) {
    return ArcInvite(
      inviteId: (json['invite_id'] as String?) ?? '',
      inviteToken: (json['invite_token'] as String?) ?? '',
      expiresAt: DateTime.parse(json['expires_at'] as String).toUtc(),
    );
  }
}

class ProfileDraft {
  const ProfileDraft({this.displayName, this.username, this.bio});

  final String? displayName;
  final String? username;
  final String? bio;

  bool get isEmpty {
    return (displayName == null || displayName!.trim().isEmpty) &&
        (username == null || username!.trim().isEmpty) &&
        (bio == null || bio!.trim().isEmpty);
  }

  ProfileDraft copyWith({String? displayName, String? username, String? bio}) {
    return ProfileDraft(
      displayName: displayName ?? this.displayName,
      username: username ?? this.username,
      bio: bio ?? this.bio,
    );
  }

  Map<String, dynamic> toJson() {
    return {'display_name': displayName, 'username': username, 'bio': bio};
  }

  String encode() => jsonEncode(toJson());

  static ProfileDraft decode(String raw) {
    final data = jsonDecode(raw) as Map<String, dynamic>;
    return ProfileDraft(
      displayName: data['display_name'] as String?,
      username: data['username'] as String?,
      bio: data['bio'] as String?,
    );
  }
}

class AuthBootstrapResult {
  const AuthBootstrapResult._({
    required this.authenticated,
    this.user,
    this.session,
    this.profileDraft,
  });

  final bool authenticated;
  final ArcUser? user;
  final ArcSession? session;
  final ProfileDraft? profileDraft;

  factory AuthBootstrapResult.authenticated({
    required ArcUser user,
    required ArcSession session,
    ProfileDraft? profileDraft,
  }) {
    return AuthBootstrapResult._(
      authenticated: true,
      user: user,
      session: session,
      profileDraft: profileDraft,
    );
  }

  factory AuthBootstrapResult.unauthenticated({ProfileDraft? profileDraft}) {
    return AuthBootstrapResult._(
      authenticated: false,
      profileDraft: profileDraft,
    );
  }
}

DateTime? _parseOptionalTime(Object? raw) {
  final s = (raw as String?)?.trim();
  if (s == null || s.isEmpty) {
    return null;
  }
  return DateTime.tryParse(s)?.toUtc();
}
