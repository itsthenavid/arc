import 'dart:async';

import 'package:arc/features/auth/data/api_exception.dart';
import 'package:arc/features/auth/data/auth_api.dart';
import 'package:arc/features/auth/data/token_store.dart';
import 'package:arc/features/auth/domain/auth_models.dart';

abstract class AuthRepository {
  ArcSession? get currentSession;
  ArcUser? get currentUser;

  Future<AuthBootstrapResult> bootstrap();

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

  Future<ArcSession> ensureActiveSession({Duration skew});

  Future<ArcSession> refreshSession();

  Future<ArcUser> refreshCurrentUser();

  Future<ArcInvite> createInvite({int maxUses, Duration? ttl});

  Future<void> applyProfileDraft(ProfileDraft draft);

  Future<void> saveProfileDraft(ProfileDraft draft);

  Future<void> clearProfileDraft();

  Future<String?> readInviteToken();

  Future<void> saveInviteToken(String token);

  Future<void> clearInviteToken();

  Future<void> logout();

  Future<void> logoutAll();
}

class DefaultAuthRepository implements AuthRepository {
  DefaultAuthRepository({
    required AuthApiClient api,
    required AuthStore store,
    required bool webCookieMode,
  }) : _api = api,
       _store = store,
       _webCookieMode = webCookieMode;

  final AuthApiClient _api;
  final AuthStore _store;
  final bool _webCookieMode;

  ArcSession? _session;
  ArcUser? _user;
  ProfileDraft? _profileDraft;

  Completer<ArcSession>? _refreshCompleter;

  @override
  ArcSession? get currentSession => _session;

  @override
  ArcUser? get currentUser => _user;

  @override
  Future<AuthBootstrapResult> bootstrap() async {
    _session ??= await _store.readSession();
    _user ??= await _store.readUser();
    _profileDraft ??= await _store.readProfileDraft();

    final existingSession = _session;
    if (existingSession == null) {
      return AuthBootstrapResult.unauthenticated(profileDraft: _profileDraft);
    }

    if (!existingSession.isAccessExpired(skew: const Duration(seconds: 30)) &&
        _user != null) {
      return AuthBootstrapResult.authenticated(
        user: _user!,
        session: existingSession,
        profileDraft: _profileDraft,
      );
    }

    try {
      await ensureActiveSession(skew: const Duration(seconds: 30));
      await refreshCurrentUser();
      return AuthBootstrapResult.authenticated(
        user: _user!,
        session: _session!,
        profileDraft: _profileDraft,
      );
    } on ApiException catch (e) {
      // Keep local state on transient faults to avoid forced logout when offline.
      if (e.isTransient && _user != null && _session != null) {
        return AuthBootstrapResult.authenticated(
          user: _user!,
          session: _session!,
          profileDraft: _profileDraft,
        );
      }
      if (e.isAuthFailure) {
        await _clearIdentityState();
      }
      return AuthBootstrapResult.unauthenticated(profileDraft: _profileDraft);
    } catch (_) {
      await _clearIdentityState();
      return AuthBootstrapResult.unauthenticated(profileDraft: _profileDraft);
    }
  }

  @override
  Future<ArcAuthBundle> login({
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) async {
    final bundle = await _api.login(
      username: username,
      email: email,
      password: password,
      rememberMe: rememberMe,
    );
    await _persistAuthBundle(bundle);
    return bundle;
  }

  @override
  Future<ArcAuthBundle> consumeInvite({
    required String inviteToken,
    String? username,
    String? email,
    required String password,
    required bool rememberMe,
  }) async {
    final bundle = await _api.consumeInvite(
      inviteToken: inviteToken,
      username: username,
      email: email,
      password: password,
      rememberMe: rememberMe,
    );

    await _persistAuthBundle(bundle);
    await clearInviteToken();
    return bundle;
  }

  @override
  Future<ArcSession> ensureActiveSession({
    Duration skew = Duration.zero,
  }) async {
    final existing = _session ?? await _store.readSession();
    if (existing == null) {
      throw const ApiException(
        message: 'Not authenticated.',
        kind: ApiErrorKind.unauthorized,
      );
    }

    _session = existing;

    if (!existing.isAccessExpired(skew: skew)) {
      return existing;
    }

    return _refreshInternal();
  }

  @override
  Future<ArcSession> refreshSession() {
    return _refreshInternal();
  }

  @override
  Future<ArcUser> refreshCurrentUser() async {
    var session = await ensureActiveSession(skew: const Duration(seconds: 10));

    try {
      final user = await _api.me(accessToken: session.accessToken);
      _user = user;
      await _store.writeUser(user);
      return user;
    } on ApiException catch (e) {
      if (e.statusCode != 401) {
        rethrow;
      }

      session = await _refreshInternal();
      final user = await _api.me(accessToken: session.accessToken);
      _user = user;
      await _store.writeUser(user);
      return user;
    }
  }

  @override
  Future<ArcInvite> createInvite({int maxUses = 1, Duration? ttl}) async {
    final session = await ensureActiveSession(
      skew: const Duration(seconds: 15),
    );
    return _api.createInvite(
      accessToken: session.accessToken,
      maxUses: maxUses,
      ttl: ttl,
    );
  }

  @override
  Future<void> applyProfileDraft(ProfileDraft draft) async {
    _profileDraft = draft;
    await _store.writeProfileDraft(draft);

    final current = _user;
    if (current == null) {
      return;
    }

    _user = current.copyWith(
      displayName: _trimOrNull(draft.displayName) ?? current.displayName,
      username: _trimOrNull(draft.username) ?? current.username,
      bio: _trimOrNull(draft.bio) ?? current.bio,
    );

    await _store.writeUser(_user!);
  }

  @override
  Future<void> saveProfileDraft(ProfileDraft draft) async {
    _profileDraft = draft;
    await _store.writeProfileDraft(draft);
  }

  @override
  Future<void> clearProfileDraft() async {
    _profileDraft = null;
    await _store.clearProfileDraft();
  }

  @override
  Future<String?> readInviteToken() {
    return _store.readInviteToken();
  }

  @override
  Future<void> saveInviteToken(String token) {
    return _store.writeInviteToken(token);
  }

  @override
  Future<void> clearInviteToken() {
    return _store.clearInviteToken();
  }

  @override
  Future<void> logout() async {
    final token = _session?.accessToken;

    try {
      if (token != null && token.trim().isNotEmpty) {
        await _api.logout(accessToken: token);
      }
    } finally {
      await _clearIdentityState();
    }
  }

  @override
  Future<void> logoutAll() async {
    final token = _session?.accessToken;

    try {
      if (token != null && token.trim().isNotEmpty) {
        await _api.logoutAll(accessToken: token);
      }
    } finally {
      await _clearIdentityState();
    }
  }

  Future<ArcSession> _refreshInternal() async {
    final inFlight = _refreshCompleter;
    if (inFlight != null) {
      return inFlight.future;
    }

    final completer = Completer<ArcSession>();
    unawaited(
      completer.future.then<void>(
        (_) {},
        onError: (Object error, StackTrace stackTrace) {},
      ),
    );
    _refreshCompleter = completer;

    try {
      final current = _session ?? await _store.readSession();
      if (current == null) {
        throw const ApiException(
          message: 'Not authenticated.',
          kind: ApiErrorKind.unauthorized,
        );
      }

      final refreshed = await _api.refresh(
        refreshToken: _webCookieMode ? null : current.refreshToken,
        rememberMe: current.rememberMe,
      );

      final mergedRefreshToken = _resolveRefreshToken(
        oldSession: current,
        refreshedSession: refreshed,
      );

      final merged = refreshed.copyWith(refreshToken: mergedRefreshToken);
      _session = merged;
      await _store.writeSession(merged);

      completer.complete(merged);
      return merged;
    } catch (error, stackTrace) {
      completer.completeError(error, stackTrace);
      rethrow;
    } finally {
      if (identical(_refreshCompleter, completer)) {
        _refreshCompleter = null;
      }
    }
  }

  String? _resolveRefreshToken({
    required ArcSession oldSession,
    required ArcSession refreshedSession,
  }) {
    final refreshed = _trimOrNull(refreshedSession.refreshToken);
    if (_webCookieMode) {
      return null;
    }
    return refreshed ?? _trimOrNull(oldSession.refreshToken);
  }

  Future<void> _persistAuthBundle(ArcAuthBundle bundle) async {
    _session = bundle.session;
    _user = bundle.user;

    await Future.wait<void>([
      _store.writeSession(bundle.session),
      _store.writeUser(bundle.user),
    ]);
  }

  Future<void> _clearIdentityState() async {
    _session = null;
    _user = null;
    await Future.wait<void>([
      _store.clearSession(),
      _store.clearUser(),
      _store.clearProfileDraft(),
    ]);
  }

  String? _trimOrNull(String? value) {
    if (value == null) {
      return null;
    }
    final trimmed = value.trim();
    if (trimmed.isEmpty) {
      return null;
    }
    return trimmed;
  }
}
